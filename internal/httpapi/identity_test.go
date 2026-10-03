package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

// Fault adapters are only unit-test dependencies. Persistence semantics have a
// separate real-PostgreSQL HTTP integration test; no runtime substitute exists.
type identityFaultStore struct {
	IdentityStore
	resolveError error
	resolved     int
	mutated      int
}

func (s *identityFaultStore) ResolveSession(context.Context, []byte) (store.Session, error) {
	s.resolved++
	return store.Session{}, s.resolveError
}
func (s *identityFaultStore) RevokeSession(context.Context, []byte, bool, string, string) error {
	s.mutated++
	return nil
}
func (s *identityFaultStore) UpdateWorkspaceNameSession(context.Context, []byte, store.UpdateWorkspaceNameCommand) (store.Workspace, error) {
	s.mutated++
	return store.Workspace{}, nil
}

type syntheticFlow struct {
	auth                   identity.Authentication
	err                    error
	state, nonce, verifier string
	exchanged              int
}

func (f *syntheticFlow) AuthorizationURL(state, nonce, verifier string) string {
	f.state, f.nonce, f.verifier = state, nonce, verifier
	return "https://issuer.example/authorize?state=" + state
}
func (f *syntheticFlow) Exchange(ctx context.Context, code, verifier, nonce string, started time.Time) (identity.Authentication, error) {
	f.exchanged++
	if ctx.Err() != nil {
		return identity.Authentication{}, ctx.Err()
	}
	if code != "synthetic-one-use-code" || nonce != f.nonce || verifier != f.verifier || started.IsZero() {
		return identity.Authentication{}, identity.ErrUnauthenticated
	}
	return f.auth, f.err
}

func identityHTTP(t *testing.T, database IdentityStore, flow IdentityFlow, log io.Writer) *Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(log, nil))
	protected, err := NewIdentityHandler(database, flow, "https://marketplace.example", logger)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(Options{Ready: func(context.Context) error { return nil }, Protected: protected, Logger: logger, ReadinessTimeout: time.Second, RequestTimeout: 10 * time.Second, MaxRequestBody: 16384})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func syntheticToken(t *testing.T) string {
	t.Helper()
	token, err := identity.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthenticationFailureAndAmbiguousCredentials(t *testing.T) {
	token := syntheticToken(t)
	for _, tc := range []struct {
		name, cookie, authorization string
		dependency                  error
		want                        int
		calls                       int
	}{
		{"missing", "", "", nil, 401, 0},
		{"malformed", sessionCookie + "=secret-canary", "", nil, 401, 0},
		{"duplicate", sessionCookie + "=" + token + "; " + sessionCookie + "=" + token, "", nil, 401, 0},
		{"malformed duplicate", sessionCookie + "=" + token + "; " + sessionCookie + "=\"broken", "", nil, 401, 0},
		{"bearer ambiguity", sessionCookie + "=" + token, "Bearer secret-canary", nil, 401, 0},
		{"revoked", sessionCookie + "=" + token, "", identity.ErrUnauthenticated, 401, 1},
		{"dependency outage", sessionCookie + "=" + token, "", errors.New("provider secret-canary"), 503, 1},
		{"denial with rollback failure", sessionCookie + "=" + token, "", errors.Join(identity.ErrUnauthenticated, store.ErrUnavailable), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &identityFaultStore{resolveError: tc.dependency}
			var logs bytes.Buffer
			h := identityHTTP(t, database, &syntheticFlow{}, &logs)
			r := httptest.NewRequest("GET", "/api/v1/session", nil)
			if tc.cookie != "" {
				r.Header.Set("Cookie", tc.cookie)
			}
			if tc.authorization != "" {
				r.Header.Set("Authorization", tc.authorization)
			}
			r.Header.Set("X-User-ID", "secret-canary")
			r.Header.Set("X-Role", "admin")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || database.resolved != tc.calls {
				t.Fatalf("status=%d calls=%d", w.Code, database.resolved)
			}
			if strings.Contains(logs.String()+w.Body.String(), "secret-canary") || strings.Contains(logs.String()+w.Body.String(), token) {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestCSRFAndCommandAmbiguityCannotMutate(t *testing.T) {
	token := syntheticToken(t)
	for _, tc := range []struct {
		name, origin, proof, body string
		want                      int
	}{
		{"missing origin", "", csrfProof(token), "{}", 403},
		{"null origin", "null", csrfProof(token), "{}", 403},
		{"foreign origin", "https://attacker.example", csrfProof(token), "{}", 403},
		{"missing proof", "https://marketplace.example", "", "{}", 403},
		{"wrong proof", "https://marketplace.example", csrfProof(syntheticToken(t)), "{}", 403},
		{"forged role", "https://marketplace.example", csrfProof(token), `{"role":"admin"}`, 400},
		{"null body", "https://marketplace.example", csrfProof(token), "null", 400},
		{"two bodies", "https://marketplace.example", csrfProof(token), "{}{}", 400},
		{"valid", "https://marketplace.example", csrfProof(token), "{}", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := &identityFaultStore{}
			h := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
			r := httptest.NewRequest("POST", "/api/v1/session/logout", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-CSRF-Token", tc.proof)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if (database.mutated == 1) != (tc.want == 204) {
				t.Fatal("mutation outside accepted CSRF boundary")
			}
			if tc.want == 204 {
				c := w.Result().Cookies()
				if len(c) != 1 || c[0].MaxAge != -1 || !c[0].Secure || !c[0].HttpOnly || c[0].Domain != "" || c[0].Path != "/" {
					t.Fatal("unsafe logout cookie")
				}
			}
		})
	}
	for _, body := range []string{`{"name":"one","name":"two","expected_version":1}`, `{"name":"one","Name":"two","expected_version":1}`, `{"name":"ok","expected_version":1,"workspace_id":"foreign"}`, `{"name":"ok","expected_version":1,"owner_id":"foreign"}`} {
		database := &identityFaultStore{}
		h := identityHTTP(t, database, &syntheticFlow{}, io.Discard)
		r := httptest.NewRequest("PATCH", "/api/v1/workspaces/10000000-0000-4000-8000-000000000001", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://marketplace.example")
		r.Header.Set("X-CSRF-Token", csrfProof(token))
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 || database.mutated != 0 {
			t.Fatal("ambiguous or authority-bearing command accepted")
		}
	}
}

func TestProtocolRoutesRejectLoginCSRFAndMalformedCallback(t *testing.T) {
	for _, tc := range []struct {
		method, path, origin string
		want                 int
	}{
		{"GET", "/api/v1/auth/start", "https://marketplace.example", 405},
		{"POST", "/api/v1/auth/start", "", 403},
		{"POST", "/api/v1/auth/start", "https://attacker.example", 403},
		{"GET", "/api/v1/auth/callback?state=x&state=y&code=x", "", 401},
		{"GET", "/api/v1/auth/callback?state=x&code=y&error=access_denied", "", 401},
		{"GET", "/api/v1/auth/callback?state=x&code=y&return_url=https://attacker.example", "", 401},
	} {
		database := &identityFaultStore{}
		flow := &syntheticFlow{}
		h := identityHTTP(t, database, flow, io.Discard)
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || flow.exchanged != 0 || database.resolved != 0 {
			t.Fatalf("malformed protocol request status=%d", w.Code)
		}
	}
}

func TestSessionProjectionExcludesExternalClaimsAndCredential(t *testing.T) {
	s := store.Session{Principal: identity.Principal{PrincipalID: "principal", UserID: "user", SessionID: "session", Kind: identity.Human, Authentication: identity.Authentication{Issuer: "secret-canary", Subject: "secret-canary", Provider: "secret-canary"}}}
	encoded, err := json.Marshal(sessionResponse(s, "csrf-proof"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-canary") || !strings.Contains(string(encoded), `"actor_kind":"HUMAN"`) {
		t.Fatal("unsafe principal projection")
	}
}
