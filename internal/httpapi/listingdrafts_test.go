package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/config"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/listings"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

const draftHTTPWorkspace = "81000000-0000-4000-8000-000000000001"
const draftHTTPBusiness = "82000000-0000-4000-8000-000000000001"
const draftHTTPListing = "83000000-0000-4000-8000-000000000001"
const draftHTTPCommand = "84000000-0000-4000-8000-000000000001"

// Transport fault dependency only. Real persistence/authorization evidence is
// exercised through restricted credentials in listingdrafts_integration_test.go.
type draftFaultStore struct {
	IdentityStore
	resolveErr, operationErr error
	resolved, calls          int
	op                       string
	replayed                 bool
	hash                     []byte
	create                   listings.CreateDraftCommand
	update                   listings.UpdateDraftCommand
	workspace, listing       string
	contextErr               error
	context                  context.Context
}

func (s *draftFaultStore) ResolveSession(context.Context, []byte) (store.Session, error) {
	s.resolved++
	return store.Session{}, s.resolveErr
}
func (s *draftFaultStore) capture(ctx context.Context, hash []byte, op string) {
	s.calls++
	s.op, s.hash, s.contextErr = op, append([]byte(nil), hash...), ctx.Err()
	s.context = ctx
}
func (s *draftFaultStore) CreateListingDraftSession(ctx context.Context, hash []byte, command listings.CreateDraftCommand) (listings.CreatedDraft, error) {
	s.capture(ctx, hash, "create")
	s.create = command
	return s.created(), s.operationErr
}
func (s *draftFaultStore) CreatePersonalListingDraftSession(ctx context.Context, hash []byte, command listings.CreateDraftCommand) (listings.CreatedDraft, error) {
	s.capture(ctx, hash, "personal")
	s.create = command
	return s.created(), s.operationErr
}
func (s *draftFaultStore) created() listings.CreatedDraft {
	return listings.CreatedDraft{WorkspaceID: draftHTTPWorkspace, BusinessID: draftHTTPBusiness, ListingID: draftHTTPListing, BusinessVersion: 1, ListingVersion: 1, Replayed: s.replayed}
}
func (s *draftFaultStore) ReadListingDraftSession(ctx context.Context, hash []byte, workspace, listing string) (listings.Draft, error) {
	s.capture(ctx, hash, "read")
	s.workspace, s.listing = workspace, listing
	return listings.Draft{}, s.operationErr
}
func (s *draftFaultStore) UpdateListingDraftSession(ctx context.Context, hash []byte, command listings.UpdateDraftCommand) (listings.Draft, error) {
	s.capture(ctx, hash, "update")
	s.update = command
	return listings.Draft{}, s.operationErr
}
func (s *draftFaultStore) PreviewListingDraftSession(ctx context.Context, hash []byte, workspace, listing string) (listings.PrivatePreview, error) {
	s.capture(ctx, hash, "preview")
	s.workspace, s.listing = workspace, listing
	return listings.PrivatePreview{}, s.operationErr
}

func draftHTTPPath(suffix string) string {
	return "/api/v1/workspaces/" + draftHTTPWorkspace + "/listing-drafts" + suffix
}
func draftHTTPRequest(method, path, body, token string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	}
	if method != http.MethodGet && method != http.MethodHead {
		r.Header.Set("Origin", "https://marketplace.example")
		r.Header.Set("X-CSRF-Token", csrfProof(token))
	}
	return r
}
func draftErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code, RequestID string
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	evidence, ok := raw["error"].(map[string]any)
	if !ok || evidence["request_id"] != response.Header().Get("X-Request-ID") || evidence["request_id"] == "" {
		t.Fatal("error lacks matching server request context")
	}
	return body.Error.Code
}

func TestListingDraftHTTPRejectsCredentialsBeforeStateCalls(t *testing.T) {
	token := syntheticToken(t)
	for _, tc := range []struct {
		name, cookie, authorization string
		dependency                  error
		want, resolves              int
	}{
		{"missing", "", "", nil, 401, 0},
		{"invalid", sessionCookie + "=credential-canary", "", nil, 401, 0},
		{"duplicate", sessionCookie + "=" + token + "; " + sessionCookie + "=" + token, "", nil, 401, 0},
		{"ambiguous bearer", sessionCookie + "=" + token, "Bearer credential-canary", nil, 401, 0},
		{"revoked", sessionCookie + "=" + token, "", identity.ErrUnauthenticated, 401, 1},
		{"dependency failure", sessionCookie + "=" + token, "", errors.New("dependency credential-canary"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &draftFaultStore{resolveErr: tc.dependency}
			var logs bytes.Buffer
			handler := identityHTTP(t, database, &syntheticFlow{}, &logs)
			r := draftHTTPRequest(http.MethodGet, draftHTTPPath("/"+draftHTTPListing+"/preview"), "", "")
			if tc.cookie != "" {
				r.Header.Set("Cookie", tc.cookie)
			}
			if tc.authorization != "" {
				r.Header.Set("Authorization", tc.authorization)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != tc.want || database.resolved != tc.resolves || database.calls != 0 {
				t.Fatalf("status=%d resolves=%d state_calls=%d", response.Code, database.resolved, database.calls)
			}
			if strings.Contains(response.Body.String()+logs.String(), "credential-canary") || strings.Contains(response.Body.String()+logs.String(), token) {
				t.Fatal("credential or dependency detail leaked")
			}
		})
	}
}

func TestListingDraftHTTPRejectsCSRFAmbiguityBeforeStateCalls(t *testing.T) {
	token := syntheticToken(t)
	for _, name := range []string{"missing origin", "foreign origin", "duplicate origin", "missing proof", "wrong proof", "duplicate proof"} {
		t.Run(name, func(t *testing.T) {
			database := &draftFaultStore{}
			handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			r := draftHTTPRequest(http.MethodPost, "/api/v1/personal-listing-drafts", `{"command_id":"`+draftHTTPCommand+`"}`, token)
			switch name {
			case "missing origin":
				r.Header.Del("Origin")
			case "foreign origin":
				r.Header.Set("Origin", "https://attacker.example")
			case "duplicate origin":
				r.Header.Add("Origin", "https://marketplace.example")
			case "missing proof":
				r.Header.Del("X-CSRF-Token")
			case "wrong proof":
				r.Header.Set("X-CSRF-Token", csrfProof(syntheticToken(t)))
			case "duplicate proof":
				r.Header.Add("X-CSRF-Token", csrfProof(token))
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != 403 || database.calls != 0 || draftErrorCode(t, response) != "csrf_rejected" {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, database.calls, response.Body.String())
			}
		})
	}
}

func TestListingDraftHTTPRejectsMalformedBodiesBeforeStateCalls(t *testing.T) {
	token := syntheticToken(t)
	base := `{"command_id":"` + draftHTTPCommand + `"`
	for _, tc := range []struct{ name, body string }{
		{"unknown owner", base + `,"owner_id":"foreign"}`},
		{"unknown workspace", base + `,"workspace_id":"foreign"}`},
		{"unknown role", base + `,"role":"admin"}`},
		{"unknown permission", base + `,"permission":"listing.update"}`},
		{"unknown state", base + `,"state":"PUBLISHED"}`},
		{"nested body", base + `,"business":{"private_label":"nested"}}`},
		{"case alias", base + `,"Private_Label":"alias"}`},
		{"duplicate", base + `,"title":"one","title":"two"}`},
		{"duplicate command", base + `,"command_id":"` + draftHTTPCommand + `"}`},
		{"null field", base + `,"title":null}`},
		{"null command", `{"command_id":null}`},
		{"null root", "null"},
		{"array root", "[]"},
		{"trailing object", base + `}{}`},
		{"trailing text", base + `}canary`},
		{"oversized", base + `,"private_label":"` + strings.Repeat("x", 20000) + `"}`},
		{"invalid utf8", base + `,"title":"` + string([]byte{0xff}) + `"}`},
		{"lone high surrogate", base + `,"title":"\ud800"}`},
		{"lone low surrogate", base + `,"title":"\udc00"}`},
		{"broken pair", base + `,"title":"\ud800\u0041"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &draftFaultStore{}
			handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, draftHTTPRequest(http.MethodPost, "/api/v1/personal-listing-drafts", tc.body, token))
			if response.Code != 400 || database.calls != 0 || draftErrorCode(t, response) != "invalid_request" {
				t.Fatalf("status=%d state_calls=%d body=%s", response.Code, database.calls, response.Body.String())
			}
		})
	}
	for _, body := range []string{
		`{"expected_business_version":null,"expected_listing_version":1}`,
		`{"expected_business_version":1,"Expected_Listing_Version":1}`,
		`{"expected_business_version":1,"expected_listing_version":1,"expected_listing_version":2}`,
		`{"expected_business_version":1.5,"expected_listing_version":1}`,
	} {
		database := &draftFaultStore{}
		handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, draftHTTPRequest(http.MethodPut, draftHTTPPath("/"+draftHTTPListing), body, token))
		if response.Code != 400 || database.calls != 0 {
			t.Fatalf("replacement ambiguity reached state boundary: status=%d calls=%d", response.Code, database.calls)
		}
	}
}

func TestListingDraftHTTPRejectsAmbiguousMediaTypesBeforeStateCalls(t *testing.T) {
	token := syntheticToken(t)
	for _, values := range [][]string{nil, {"text/plain"}, {"application/json; bad"}, {"application/json", "application/json"}} {
		database := &draftFaultStore{}
		handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
		r := draftHTTPRequest(http.MethodPost, "/api/v1/personal-listing-drafts", `{"command_id":"`+draftHTTPCommand+`"}`, token)
		r.Header.Del("Content-Type")
		for _, value := range values {
			r.Header.Add("Content-Type", value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != 400 || database.calls != 0 || draftErrorCode(t, response) != "invalid_request" {
			t.Fatalf("ambiguous media type reached command boundary: status=%d calls=%d", response.Code, database.calls)
		}
	}
}

func TestListingDraftHTTPConfiguredLimitCarriesMaximumUnicodeFields(t *testing.T) {
	// Load the actual bounded request configuration, independent of process env.
	// Only transport settings are adopted; the explicit test identity dependency
	// is composed below, never selected by production disabled-mode composition.
	settings := map[string]string{"AUTHENTICATION_MODE": "disabled", "DATABASE_URL": "postgres://synthetic@localhost/foundation?sslmode=disable", "HTTP_ADDR": "127.0.0.1:8080"}
	cfg, err := config.Load(func(key string) string { return settings[key] }, config.API)
	if err != nil {
		t.Fatal(err)
	}
	fields := listings.Fields{PrivateLabel: strings.Repeat("😀", 120), ActivityDescription: strings.Repeat("😀", 2000),
		Country: "SE", Region: strings.Repeat("😀", 120), Title: strings.Repeat("😀", 160), Description: strings.Repeat("😀", 8000),
		SaleSubject: "business_division", TransferStructure: "business_transfer", SaleContext: "restructuring", SaleMethod: "time_limited_bidding"}
	if _, err := fields.Normalize(); err != nil {
		t.Fatal("maximum Unicode fixture violates finite domain limits", err)
	}
	body, err := json.Marshal(struct {
		CommandID string `json:"command_id"`
		listings.Fields
	}{draftHTTPCommand, fields})
	if err != nil {
		t.Fatal(err)
	}
	// Exercise worst-case valid escaped representation: twelve wire bytes per
	// supplementary Unicode character, including the entire 10,000 descriptions.
	escaped := strings.ReplaceAll(string(body), "😀", `\ud83d\ude00`)
	if int64(len(escaped)) > cfg.MaxRequestBody {
		t.Fatalf("configured body limit %d cannot carry maximum valid Unicode contract (%d bytes)", cfg.MaxRequestBody, len(escaped))
	}
	token := syntheticToken(t)
	for _, tc := range []struct {
		name, body string
		status     int
		calls      int
	}{
		{"maximum valid Unicode", escaped, http.StatusCreated, 1},
		{"configured bound exceeded", strings.Repeat(" ", int(cfg.MaxRequestBody)+1) + escaped, http.StatusBadRequest, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &draftFaultStore{}
			base := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			handler, err := New(Options{Ready: base.options.Ready, Protected: base.options.Protected, Logger: base.options.Logger,
				ReadinessTimeout: cfg.ReadinessTimeout, RequestTimeout: cfg.RequestTimeout, MaxRequestBody: cfg.MaxRequestBody})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, draftHTTPRequest(http.MethodPost, "/api/v1/personal-listing-drafts", tc.body, token))
			if response.Code != tc.status || database.calls != tc.calls {
				t.Fatalf("status=%d want=%d calls=%d want=%d", response.Code, tc.status, database.calls, tc.calls)
			}
			if tc.calls == 1 && database.create.Fields != fields {
				t.Fatal("maximum valid Unicode text changed at the authentication/transport boundary")
			}
		})
	}
}

func TestListingDraftHTTPRejectsUnknownQueryAndMethodsBeforeStateCalls(t *testing.T) {
	token := syntheticToken(t)
	for _, tc := range []struct {
		method, path, allow string
		status              int
	}{
		{http.MethodGet, draftHTTPPath("/" + draftHTTPListing + "?workspace_id=foreign"), "", 400},
		{http.MethodPost, "/api/v1/personal-listing-drafts?owner_id=foreign", "", 400},
		{http.MethodGet, draftHTTPPath("/" + draftHTTPListing + "?"), "", 400},
		{http.MethodGet, "/api/v1/personal-listing-drafts", "POST", 405},
		{http.MethodPut, draftHTTPPath(""), "POST", 405},
		{http.MethodPatch, draftHTTPPath("/" + draftHTTPListing), "GET, PUT", 405},
		{http.MethodHead, draftHTTPPath("/" + draftHTTPListing), "GET, PUT", 405},
		{http.MethodPut, draftHTTPPath("/" + draftHTTPListing + "/preview"), "GET", 405},
		{http.MethodGet, draftHTTPPath("/" + draftHTTPListing + "/unknown"), "", 404},
		{http.MethodGet, "/api/v1/workspaces/short/listing-drafts/short", "", 404},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			database := &draftFaultStore{}
			handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, draftHTTPRequest(tc.method, tc.path, "{}", token))
			if response.Code != tc.status || database.calls != 0 || response.Header().Get("Allow") != tc.allow {
				t.Fatalf("status=%d calls=%d allow=%q", response.Code, database.calls, response.Header().Get("Allow"))
			}
		})
	}
}

func TestListingDraftHTTPReplacementPreservesContextAndOmission(t *testing.T) {
	token := syntheticToken(t)
	database := &draftFaultStore{}
	handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
	type requestContextKey struct{}
	ctx := context.WithValue(context.Background(), requestContextKey{}, "synthetic-context")
	r := draftHTTPRequest(http.MethodPut, draftHTTPPath("/"+draftHTTPListing), `{"expected_business_version":2,"expected_listing_version":3,"title":"Replacement"}`, token).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if response.Code != 200 || database.calls != 1 || database.op != "update" || database.update.WorkspaceID != draftHTTPWorkspace || database.update.DraftID != draftHTTPListing || database.update.ExpectedBusinessVersion != 2 || database.update.ExpectedListingVersion != 3 || database.update.Fields != (listings.Fields{Title: "Replacement"}) {
		t.Fatalf("replacement context or omitted fields changed: status=%d command=%+v", response.Code, database.update)
	}
	deadline, bounded := database.context.Deadline()
	if !bounded || deadline.After(time.Now().Add(11*time.Second)) || database.context.Value(requestContextKey{}) != "synthetic-context" {
		t.Fatal("request dependency lost parent context or request timeout")
	}
	if database.update.RequestID != response.Header().Get("X-Request-ID") || database.update.CorrelationID != database.update.RequestID {
		t.Fatal("replacement lacks server-owned audit context")
	}
}

func TestListingDraftHTTPCommandsDeriveContextAndCreationReplayStatus(t *testing.T) {
	token := syntheticToken(t)
	hash, err := identity.HashToken(token)
	if err != nil {
		t.Fatal(err)
	}
	for _, personal := range []bool{false, true} {
		for _, replayed := range []bool{false, true} {
			database := &draftFaultStore{replayed: replayed}
			handler := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			path, operation, workspace := draftHTTPPath(""), "create", draftHTTPWorkspace
			if personal {
				path, operation, workspace = "/api/v1/personal-listing-drafts", "personal", ""
			}
			r := draftHTTPRequest(http.MethodPost, path, `{"command_id":"`+draftHTTPCommand+`","private_label":"synthetic private","title":"\ud83d\ude00"}`, token)
			r.Header.Set("X-Actor-ID", "actor-canary")
			r.Header.Set("X-Workspace-ID", "foreign-workspace-canary")
			r.Header.Set("X-Request-ID", "forged-request-canary")
			r.Header.Set("X-Correlation-ID", "forged-correlation-canary")
			r.Header.Set("X-Role", "admin")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			want := 201
			if replayed {
				want = 200
			}
			if response.Code != want || database.calls != 1 || database.op != operation || database.create.WorkspaceID != workspace || database.create.CommandID != draftHTTPCommand || !bytes.Equal(database.hash, hash) {
				t.Fatalf("unexpected trusted command boundary: status=%d calls=%d command=%+v", response.Code, database.calls, database.create)
			}
			if database.create.RequestID == "" || database.create.RequestID != response.Header().Get("X-Request-ID") || database.create.CorrelationID != database.create.RequestID || database.create.RequestID == "forged-request-canary" {
				t.Fatal("client request/correlation context became authoritative")
			}
			if database.create.Fields.Title != "😀" || database.create.Fields.PrivateLabel != "synthetic private" {
				t.Fatal("valid field or surrogate pair was corrupted")
			}
			if strings.Contains(response.Body.String(), "synthetic private") || strings.Contains(response.Body.String(), token) {
				t.Fatal("create outcome contains private input or credential")
			}
		}
	}
}

func TestListingDraftHTTPMapsFaultsWithoutLeakingPrivateContext(t *testing.T) {
	token := syntheticToken(t)
	for _, tc := range []struct {
		name, method, path, body, code string
		err                            error
		status                         int
	}{
		{"denied", "GET", draftHTTPPath("/" + draftHTTPListing), "", "not_found", store.ErrNotFound, 404},
		{"transaction session revoked", "GET", draftHTTPPath("/" + draftHTTPListing + "/preview"), "", "authentication_required", identity.ErrUnauthenticated, 401},
		{"stale replacement", "PUT", draftHTTPPath("/" + draftHTTPListing), `{"expected_business_version":1,"expected_listing_version":1}`, "version_conflict", store.ErrConflict, 409},
		{"command mismatch", "POST", "/api/v1/personal-listing-drafts", `{"command_id":"` + draftHTTPCommand + `"}`, "command_conflict", store.ErrConflict, 409},
		{"commit uncertainty over conflict", "POST", "/api/v1/personal-listing-drafts", `{"command_id":"` + draftHTTPCommand + `"}`, "dependency_unavailable", errors.Join(store.ErrConflict, store.ErrUnavailable), 503},
		{"rollback uncertainty over denial", "GET", draftHTTPPath("/" + draftHTTPListing), "", "dependency_unavailable", errors.Join(store.ErrNotFound, store.ErrUnavailable), 503},
		{"dependency detail", "GET", draftHTTPPath("/" + draftHTTPListing), "", "dependency_unavailable", errors.New("database token-private-canary"), 503},
		{"cancellation", "GET", draftHTTPPath("/" + draftHTTPListing), "", "dependency_unavailable", context.Canceled, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &draftFaultStore{operationErr: tc.err}
			var logs bytes.Buffer
			handler := identityHTTP(t, database, &syntheticFlow{}, &logs)
			r := draftHTTPRequest(tc.method, tc.path, tc.body, token)
			r.Header.Set("X-Role", "token-private-canary")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != tc.status || database.calls != 1 || draftErrorCode(t, response) != tc.code {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, database.calls, response.Body.String())
			}
			if strings.Contains(response.Body.String()+logs.String(), "token-private-canary") || strings.Contains(response.Body.String()+logs.String(), token) {
				t.Fatal("credential/private dependency detail leaked")
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("private response lost transport protection")
			}
		})
	}
}
