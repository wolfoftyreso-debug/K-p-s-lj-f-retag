package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/listings"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

// This exercises the HTTP contract with the real restricted API role, migrations,
// session resolver, RLS, receipts and audit constraints. Authentication facts are
// synthetic; the production identity adapter has separate protocol tests.
func TestListingDraftHTTPAgainstPostgreSQL(t *testing.T) {
	admin, database := identityDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	login := func(t *testing.T, subject string) (string, store.Session) {
		t.Helper()
		token := syntheticToken(t)
		hash, err := identity.HashToken(token)
		if err != nil {
			t.Fatal(err)
		}
		auth := identity.Authentication{Provider: "synthetic", Issuer: "https://issuer.example/", Subject: subject,
			EmailVerified: true, AuthenticatedAt: time.Now().UTC(), Assurance: identity.Assurance{Level: identity.AssuranceUnknown}}
		session, err := database.CompleteLogin(ctx, auth, hash, nil, time.Now().UTC().Add(-time.Second), draftHTTPCommand, draftHTTPCommand)
		if err != nil {
			t.Fatal(err)
		}
		return token, session
	}
	tokenA, sessionA := login(t, "synthetic-draft-owner")
	tokenB, sessionB := login(t, "synthetic-draft-other")
	handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := draftHTTPRequest(method, path, body, token).WithContext(ctx)
		r.Header.Set("X-Actor-ID", sessionB.Principal.UserID)
		r.Header.Set("X-Workspace-ID", draftHTTPWorkspace)
		r.Header.Set("X-Role", "admin")
		r.Header.Set("X-Request-ID", "forged-request-canary")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	assertStatus := func(t *testing.T, response *httptest.ResponseRecorder, want int) {
		t.Helper()
		if response.Code != want {
			t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("protected response lost cache/content protections")
		}
	}
	createdOutcome := func(t *testing.T, response *httptest.ResponseRecorder) listings.CreatedDraft {
		t.Helper()
		var outcome listings.CreatedDraft
		if err := json.Unmarshal(response.Body.Bytes(), &outcome); err != nil {
			t.Fatal(err)
		}
		if outcome.WorkspaceID == "" || outcome.BusinessID == "" || outcome.ListingID == "" ||
			outcome.BusinessID == outcome.ListingID || outcome.BusinessVersion != 1 || outcome.ListingVersion != 1 {
			t.Fatal("create outcome lacks distinct initial aggregate identities/versions")
		}
		return outcome
	}
	decodeDraft := func(t *testing.T, response *httptest.ResponseRecorder) listings.Draft {
		t.Helper()
		var draft listings.Draft
		if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
			t.Fatal(err)
		}
		return draft
	}
	counts := func(t *testing.T, wantBusinesses, wantDrafts, wantReceipts, wantAudits int) {
		t.Helper()
		for _, tc := range []struct {
			query string
			want  int
		}{
			{`SELECT count(*) FROM listings.businesses`, wantBusinesses},
			{`SELECT count(*) FROM listings.drafts`, wantDrafts},
			{`SELECT count(*) FROM listings.command_receipts`, wantReceipts},
			{`SELECT count(*) FROM audit.aggregate_events`, wantAudits},
		} {
			var actual int
			if err := admin.QueryRow(ctx, tc.query).Scan(&actual); err != nil || actual != tc.want {
				t.Fatalf("persistent count=%d want=%d error=%v", actual, tc.want, err)
			}
		}
	}

	const personalPath = "/api/v1/personal-listing-drafts"
	const createBody = `{"command_id":"84000000-0000-4000-8000-000000000001","private_label":"private-label-canary","activity_description":"Synthetic workshop","country":" se ","region":"Synthetic region","title":"Synthetic title","description":"Synthetic description","sale_subject":"operating_business","transfer_structure":"business_transfer","sale_context":"succession","sale_method":"negotiation"}`
	first := request("POST", personalPath, createBody, tokenA)
	assertStatus(t, first, 201)
	created := createdOutcome(t, first)
	if created.Replayed || strings.Contains(first.Body.String(), "private-label-canary") {
		t.Fatal("first create is a replay or leaks private input")
	}
	resource := "/api/v1/workspaces/" + created.WorkspaceID + "/listing-drafts/" + created.ListingID
	counts(t, 1, 1, 1, 3)
	var attributed, assignments int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit.aggregate_events WHERE actor_kind='HUMAN' AND actor_user_id=$1 AND actor_session_id=$2 AND workspace_id=$3 AND request_id=$4 AND correlation_id=$4 AND result='SUCCESS' AND metadata=jsonb_build_object('previous_version',previous_version,'version',resource_version)`,
		sessionA.Principal.UserID, sessionA.Principal.SessionID, created.WorkspaceID, first.Header().Get("X-Request-ID")).Scan(&attributed); err != nil || attributed != 3 {
		t.Fatalf("audit actor/context attribution=%d error=%v", attributed, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM organizations.personal_drafting_assignments a JOIN organizations.workspaces w ON w.id=a.workspace_id WHERE a.user_id=$1 AND w.owner_kind='PERSON' AND w.owner_user_id=$1 AND w.owner_organization_id IS NULL`, sessionA.Principal.UserID).Scan(&assignments); err != nil || assignments != 1 {
		t.Fatal("personal bootstrap lacks explicit principal-derived ownership")
	}

	t.Run("stable retry and normalized fingerprint", func(t *testing.T) {
		retry := request("POST", personalPath, createBody, tokenA)
		assertStatus(t, retry, 200)
		outcome := createdOutcome(t, retry)
		if !outcome.Replayed || outcome.WorkspaceID != created.WorkspaceID || outcome.BusinessID != created.BusinessID || outcome.ListingID != created.ListingID {
			t.Fatal("retry created a different outcome")
		}
		normalized := strings.Replace(createBody, `"country":" se "`, `"country":"SE"`, 1)
		assertStatus(t, request("POST", personalPath, normalized, tokenA), 200)
		conflict := request("POST", personalPath, strings.Replace(createBody, "Synthetic title", "Different title", 1), tokenA)
		assertStatus(t, conflict, 409)
		if draftErrorCode(t, conflict) != "command_conflict" {
			t.Fatal("create mismatch lacks deterministic command conflict")
		}
		counts(t, 1, 1, 1, 3)
	})

	t.Run("private read and protected allowlisted preview", func(t *testing.T) {
		read := request("GET", resource, "", tokenA)
		assertStatus(t, read, 200)
		draft := decodeDraft(t, read)
		if draft.Business.PrivateLabel != "private-label-canary" || draft.Business.Country != "SE" || draft.Listing.Status != "DRAFT" || draft.Business.Version != 1 || draft.Listing.Version != 1 {
			t.Fatal("private persisted draft did not retain normalized fields")
		}
		preview := request("GET", resource+"/preview", "", tokenA)
		assertStatus(t, preview, 200)
		if preview.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Fatal("protected preview missing indexing exclusion")
		}
		for _, excluded := range []string{"private-label-canary", "private_label", "workspace_id", "business_id", "owner", "actor", "credential", "session", "command_id", created.BusinessID, created.WorkspaceID, tokenA} {
			if strings.Contains(preview.Body.String(), excluded) {
				t.Fatalf("protected preview includes excluded field/value %q", excluded)
			}
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(preview.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{"listing_id": true, "business_version": true, "listing_version": true, "status": true,
			"activity_description": true, "country": true, "region": true, "title": true, "description": true,
			"sale_subject": true, "transfer_structure": true, "sale_context": true, "sale_method": true}
		if len(raw) != len(allowed) {
			t.Fatal("protected preview does not match the reviewed finite contract")
		}
		for key := range raw {
			if !allowed[key] {
				t.Fatalf("protected preview has unreviewed key %q", key)
			}
		}
		var projection listings.PrivatePreview
		if err := json.Unmarshal(preview.Body.Bytes(), &projection); err != nil || projection.ListingID != created.ListingID || projection.BusinessVersion != 1 || projection.ListingVersion != 1 || projection.Title != "Synthetic title" || projection.SaleSubject != "operating_business" {
			t.Fatal("preview lost source versions or explicit presentation")
		}
		counts(t, 1, 1, 1, 3)
	})

	t.Run("negative ownership and credentials", func(t *testing.T) {
		for _, tc := range []struct {
			method, path, body, token string
			want                      int
		}{
			{"GET", resource, "", "", 401},
			{"GET", resource + "/preview", "", "malformed-token", 401},
			{"GET", resource, "", tokenB, 404},
			{"GET", resource + "/preview", "", tokenB, 404},
			{"PUT", resource, `{"expected_business_version":1,"expected_listing_version":1,"title":"stolen"}`, tokenB, 404},
			{"GET", strings.Replace(resource, created.WorkspaceID, draftHTTPWorkspace, 1), "", tokenA, 404},
			{"GET", strings.Replace(resource, created.ListingID, draftHTTPListing, 1), "", tokenA, 404},
			{"POST", personalPath, `{"command_id":"` + draftHTTPCommand + `","owner_id":"` + sessionB.Principal.UserID + `"}`, tokenA, 400},
			{"POST", personalPath, `{}`, tokenA, 400},
			{"PUT", resource, `{"expected_listing_version":1}`, tokenA, 400},
			{"PUT", resource, `{"expected_business_version":1}`, tokenA, 400},
			{"PUT", resource, `{"expected_business_version":1,"expected_listing_version":1,"sale_method":"auction"}`, tokenA, 400},
		} {
			response := request(tc.method, tc.path, tc.body, tc.token)
			assertStatus(t, response, tc.want)
			if tc.want == 404 && draftErrorCode(t, response) != "not_found" {
				t.Fatal("protected denial leaks resource-existence distinction")
			}
			if strings.Contains(response.Body.String(), "private-label-canary") {
				t.Fatal("denied request disclosed private state")
			}
		}
		counts(t, 1, 1, 1, 3)
	})

	t.Run("full replacement source versions and no-op", func(t *testing.T) {
		const replacement = `{"expected_business_version":1,"expected_listing_version":1,"title":"Replacement title"}`
		updated := request("PUT", resource, replacement, tokenA)
		assertStatus(t, updated, 200)
		draft := decodeDraft(t, updated)
		if draft.Business.Version != 2 || draft.Listing.Version != 2 || draft.Business.PrivateLabel != "" || draft.Business.ActivityDescription != "" || draft.Business.Country != "" || draft.Business.Region != "" || draft.Listing.Title != "Replacement title" || draft.Listing.Description != "" || draft.Listing.SaleSubject != "" || draft.Listing.Status != "DRAFT" {
			t.Fatal("PUT failed full-replacement semantics or changed an unauthorized state")
		}
		counts(t, 1, 1, 1, 5)
		stale := request("PUT", resource, replacement, tokenA)
		assertStatus(t, stale, 409)
		if draftErrorCode(t, stale) != "version_conflict" {
			t.Fatal("stale edit not reported as version conflict")
		}
		noop := request("PUT", resource, `{"expected_business_version":2,"expected_listing_version":2,"title":" Replacement title "}`, tokenA)
		assertStatus(t, noop, 200)
		if draft := decodeDraft(t, noop); draft.Business.Version != 2 || draft.Listing.Version != 2 {
			t.Fatal("normalized no-op fabricated aggregate revisions")
		}
		counts(t, 1, 1, 1, 5)
		// Receipt is the immutable creation result even after subsequent mutation.
		replayed := request("POST", personalPath, createBody, tokenA)
		assertStatus(t, replayed, 200)
		if outcome := createdOutcome(t, replayed); !outcome.Replayed || outcome.BusinessID != created.BusinessID || outcome.ListingID != created.ListingID {
			t.Fatal("updated resource corrupted immutable create receipt")
		}
		read := request("GET", resource, "", tokenA)
		assertStatus(t, read, 200)
		if draft := decodeDraft(t, read); draft.Business.Version != 2 || draft.Listing.Version != 2 || draft.Listing.Title != "Replacement title" {
			t.Fatal("receipt replay replaced current resource with initial state")
		}
	})

	t.Run("concurrent private and foreign requests", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan bool, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				token, want := tokenA, 200
				if i%2 == 1 {
					token, want = tokenB, 404
				}
				results <- request("GET", resource+"/preview", "", token).Code == want
			}(i)
		}
		wg.Wait()
		close(results)
		for valid := range results {
			if !valid {
				t.Fatal("concurrent request crossed or retained tenant pool context")
			}
		}
	})

	t.Run("organization relationship cannot bypass scoped grants", func(t *testing.T) {
		const organization = "85000000-0000-4000-8000-000000000001"
		const workspace = "86000000-0000-4000-8000-000000000001"
		const command = "87000000-0000-4000-8000-000000000001"
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.organizations(id) VALUES($1)`, organization); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.organization_memberships(organization_id,user_id) VALUES($1,$2)`, organization, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspaces(id,owner_kind,owner_organization_id,name) VALUES($1,'ORGANIZATION',$2,'Synthetic organization workspace')`, workspace, organization); err != nil {
			t.Fatal(err)
		}
		collection := "/api/v1/workspaces/" + workspace + "/listing-drafts"
		body := `{"command_id":"` + command + `"}`
		assertStatus(t, request("POST", collection, body, tokenB), 404)
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_memberships(workspace_id,user_id) VALUES($1,$2)`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES($1,$2,'workspace.read'),($1,$2,'workspace.update')`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("POST", collection, body, tokenB), 404)
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES($1,$2,'listing.create')`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("POST", collection, body, tokenB), 404)
		if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES($1,$2,'listing.read_private'),($1,$2,'listing.update')`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		response := request("POST", collection, body, tokenB)
		assertStatus(t, response, 201)
		outcome := createdOutcome(t, response)
		organizationResource := collection + "/" + outcome.ListingID
		assertStatus(t, request("GET", organizationResource, "", tokenB), 200)
		assertStatus(t, request("GET", organizationResource, "", tokenA), 404)
		if _, err := admin.Exec(ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='listing.update'`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("PUT", organizationResource, `{"expected_business_version":1,"expected_listing_version":1,"title":"revoked"}`, tokenB), 404)
		assertStatus(t, request("GET", organizationResource, "", tokenB), 200)
		if _, err := admin.Exec(ctx, `UPDATE organizations.organization_memberships SET active=false WHERE organization_id=$1 AND user_id=$2`, organization, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		// Organization relationships and explicit workspace grants have independent
		// lifecycles; revocation must target the actual authority being exercised.
		assertStatus(t, request("GET", organizationResource, "", tokenB), 200)
		if _, err := admin.Exec(ctx, `UPDATE organizations.workspace_memberships SET active=false WHERE workspace_id=$1 AND user_id=$2`, workspace, sessionB.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("GET", organizationResource, "", tokenB), 404)
		assertStatus(t, request("POST", collection, body, tokenB), 404)
		counts(t, 2, 2, 2, 7)
	})

	t.Run("expired credential and disabled account cannot bootstrap", func(t *testing.T) {
		if _, err := admin.Exec(ctx, `UPDATE identity.sessions SET authenticated_at=statement_timestamp()-interval '9 hours',absolute_expires_at=statement_timestamp()-interval '1 hour' WHERE id=$1`, sessionB.Principal.SessionID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("GET", resource+"/preview", "", tokenB), 401)
		assertStatus(t, request("POST", personalPath, createBody, tokenB), 401)
		tokenC, sessionC := login(t, "synthetic-disabled-draft-user")
		var disabled bool
		if err := admin.QueryRow(ctx, `SELECT identity.disable_account($1,$2,$2)`, sessionC.Principal.UserID, draftHTTPCommand).Scan(&disabled); err != nil || !disabled {
			t.Fatalf("synthetic operator disable failed: disabled=%t error=%v", disabled, err)
		}
		assertStatus(t, request("GET", resource, "", tokenC), 401)
		assertStatus(t, request("POST", personalPath, createBody, tokenC), 401)
		counts(t, 2, 2, 2, 7)
	})

	t.Run("personal bootstrap does not restore revoked permissions", func(t *testing.T) {
		if _, err := admin.Exec(ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='listing.create'`, created.WorkspaceID, sessionA.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("POST", personalPath, createBody, tokenA), 404)
		assertStatus(t, request("GET", resource, "", tokenA), 200)
		var grants int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='listing.create'`, created.WorkspaceID, sessionA.Principal.UserID).Scan(&grants); err != nil || grants != 0 {
			t.Fatal("personal bootstrap replenished a revoked grant")
		}
		if _, err := admin.Exec(ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='listing.read_private'`, created.WorkspaceID, sessionA.Principal.UserID); err != nil {
			t.Fatal(err)
		}
		assertStatus(t, request("GET", resource, "", tokenA), 404)
		assertStatus(t, request("GET", resource+"/preview", "", tokenA), 404)
		counts(t, 2, 2, 2, 7)
	})

	t.Run("session revocation denies all private draft paths", func(t *testing.T) {
		response := request("POST", "/api/v1/session/logout", "{}", tokenA)
		assertStatus(t, response, 204)
		assertStatus(t, request("GET", resource, "", tokenA), 401)
		assertStatus(t, request("GET", resource+"/preview", "", tokenA), 401)
		assertStatus(t, request("POST", personalPath, createBody, tokenA), 401)
		counts(t, 2, 2, 2, 7)
	})
}
