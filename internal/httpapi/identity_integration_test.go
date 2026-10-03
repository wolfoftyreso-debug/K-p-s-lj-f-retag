package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/db/migrations"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

func identityDatabase(t *testing.T) (*pgx.Conn, *store.Store) {
	t.Helper()
	adminDSN, apiDSN := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_API_DATABASE_URL")
	if adminDSN == "" || apiDSN == "" {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			t.Fatal("required PostgreSQL HTTP identity integration DSNs absent")
		}
		t.Skip("real PostgreSQL HTTP integration not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bootstrap, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal("test administrator connection failed")
	}
	t.Cleanup(func() {
		if err := bootstrap.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	randomHash, err := identity.HashToken(syntheticToken(t))
	if err != nil {
		t.Fatal(err)
	}
	name := "identity_http_" + hex.EncodeToString(randomHash[:8])
	if _, err := bootstrap.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := bootstrap.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgx.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal("invalid test configuration")
	}
	config.Database = name
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("isolated administrator connection failed")
	}
	t.Cleanup(func() {
		if err := admin.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := migrations.Apply(ctx, admin); err != nil {
		t.Fatal(err)
	}
	apiURL, err := url.Parse(apiDSN)
	if err != nil || apiURL.Host == "" {
		t.Fatal("test API DSN must use URI form")
	}
	apiURL.Path = "/" + name
	database, err := store.Open(ctx, apiURL.String(), "api")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return admin, database
}

func TestIdentityHTTPAgainstPostgreSQL(t *testing.T) {
	admin, database := identityDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	flow := &syntheticFlow{auth: identity.Authentication{Provider: "synthetic", Issuer: "https://issuer.example/", Subject: "synthetic-http-user", EmailVerified: true, AuthenticatedAt: time.Now().UTC(), Assurance: identity.Assurance{Level: identity.AssuranceUnknown}}}
	handler := identityHTTP(t, database, flow, io.Discard)
	request := func(method, path, body, token, origin, proof string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if proof != "" {
			r.Header.Set("X-CSRF-Token", proof)
		}
		// Untrusted identity and role headers must never influence SQL authorization.
		r.Header.Set("X-Actor-ID", "forged-user")
		r.Header.Set("X-Role", "admin")
		r.Header.Set("X-Workspace-ID", "forged-workspace")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	start := request("POST", "/api/v1/auth/start", "{}", "", "https://marketplace.example", "")
	if start.Code != 200 {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	bindings := start.Result().Cookies()
	if len(bindings) != 1 || !bindings[0].Secure || !bindings[0].HttpOnly || bindings[0].SameSite != http.SameSiteLaxMode || bindings[0].MaxAge != 300 {
		t.Fatal("login cookie policy")
	}
	callbackPath := "/api/v1/auth/callback?state=" + flow.state + "&code=synthetic-one-use-code"
	callback := httptest.NewRequest("GET", callbackPath, nil)
	callback.AddCookie(bindings[0])
	loggedIn := httptest.NewRecorder()
	handler.ServeHTTP(loggedIn, callback)
	if loggedIn.Code != 200 {
		t.Fatalf("callback status=%d body=%s", loggedIn.Code, loggedIn.Body.String())
	}
	var token string
	for _, cookie := range loggedIn.Result().Cookies() {
		if cookie.Name == sessionCookie {
			token = cookie.Value
			if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 0 {
				t.Fatal("session cookie policy")
			}
		}
	}
	if token == "" {
		t.Fatal("missing session")
	}
	var principal struct {
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
		CSRF      string `json:"csrf_token"`
	}
	if err := json.Unmarshal(loggedIn.Body.Bytes(), &principal); err != nil || principal.CSRF != csrfProof(token) {
		t.Fatal("invalid session projection")
	}
	replay := httptest.NewRecorder()
	replayedRequest := httptest.NewRequest("GET", callbackPath, nil)
	replayedRequest.AddCookie(bindings[0])
	handler.ServeHTTP(replay, replayedRequest)
	if replay.Code != 401 || flow.exchanged != 1 {
		t.Fatal("callback replay reached provider or succeeded")
	}
	const workspaceA = "10000000-0000-4000-8000-000000000001"
	const workspaceB = "10000000-0000-4000-8000-000000000002"
	if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,name) VALUES($1,'PERSON',$3,'Synthetic A'),($2,'PERSON',$3,'Synthetic B')`, workspaceA, workspaceB, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if response := request("GET", "/api/v1/workspaces/"+workspaceA, "", token, "", ""); response.Code != 404 {
		t.Fatal("login or owner identity conferred workspace access")
	}
	if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_memberships(workspace_id,user_id) VALUES($1,$2)`, workspaceA, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES($1,$2,'workspace.read'),($1,$2,'workspace.update')`, workspaceA, principal.UserID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"own read", "GET", workspaceA, "", 200},
		{"foreign read", "GET", workspaceB, "", 404},
		{"guessed read", "GET", "10000000-0000-4000-8000-000000000099", "", 404},
		{"foreign mutate", "PATCH", workspaceB, `{"name":"stolen","expected_version":1}`, 404},
		{"forged role body", "PATCH", workspaceA, `{"name":"stolen","expected_version":1,"role":"admin"}`, 400},
		{"own mutate", "PATCH", workspaceA, `{"name":"Updated","expected_version":1}`, 200},
		{"conflict", "PATCH", workspaceA, `{"name":"Stale","expected_version":1}`, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := request(tc.method, "/api/v1/workspaces/"+tc.path, tc.body, token, "https://marketplace.example", principal.CSRF)
			if response.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.want, response.Body.String())
			}
		})
	}
	var actor, kind string
	var auditCount, outboxCount int
	if err := admin.QueryRow(ctx, `SELECT actor_user_id::text,actor_kind FROM audit.events WHERE workspace_id=$1`, workspaceA).Scan(&actor, &kind); err != nil || actor != principal.UserID || kind != "HUMAN" {
		t.Fatal("incorrect material audit actor attribution")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit.events`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM eventing.outbox`).Scan(&outboxCount); err != nil || auditCount != 1 || outboxCount != 1 {
		t.Fatal("HTTP mutation was not atomic with single audit/outbox")
	}
	if _, err := admin.Exec(ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='workspace.update'`, workspaceA, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if response := request("PATCH", "/api/v1/workspaces/"+workspaceA, `{"name":"revoked","expected_version":2}`, token, "https://marketplace.example", principal.CSRF); response.Code != 404 {
		t.Fatal("stale session permission bypass")
	}
	t.Run("concurrent permitted and foreign requests", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan bool, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				id, want := workspaceA, 200
				if i%2 == 1 {
					id, want = workspaceB, 404
				}
				results <- request("GET", "/api/v1/workspaces/"+id, "", token, "", "").Code == want
			}(i)
		}
		wg.Wait()
		close(results)
		for ok := range results {
			if !ok {
				t.Fatal("concurrent access violated contract")
			}
		}
	})
	// A remote provider outage does not invalidate an already valid local session.
	flow.err = identity.ErrUnavailable
	if response := request("GET", "/api/v1/workspaces/"+workspaceA, "", token, "", ""); response.Code != 200 {
		t.Fatal("ordinary session unexpectedly depends on provider availability")
	}
	if response := request("POST", "/api/v1/session/revoke-all", "{}", token, "https://marketplace.example", principal.CSRF); response.Code != 204 {
		t.Fatalf("revoke all: %d", response.Code)
	}
	if response := request("GET", "/api/v1/workspaces/"+workspaceA, "", token, "", ""); response.Code != 401 {
		t.Fatal("revoked session accepted")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit.security_events WHERE action='sessions.revoked' AND actor_user_id=$1 AND actor_kind='HUMAN'`, principal.UserID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("revocation audit missing")
	}
}
