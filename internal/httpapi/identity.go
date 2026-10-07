package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

const sessionCookie = "__Host-marketplace-session"
const loginCookie = "__Host-marketplace-login"

// IdentityStore is the durable authentication and current-permission boundary.
// Workspace operations revalidate sessions inside their own PostgreSQL transaction.
type IdentityStore interface {
	ListingDraftStore
	CreateLogin(context.Context, store.LoginTransaction) error
	ConsumeLogin(context.Context, []byte, []byte) (store.LoginTransaction, error)
	CompleteLogin(context.Context, identity.Authentication, []byte, []byte, time.Time, string, string) (store.Session, error)
	ResolveSession(context.Context, []byte) (store.Session, error)
	RevokeSession(context.Context, []byte, bool, string, string) error
	ReadWorkspaceSession(context.Context, []byte, string) (store.Workspace, error)
	UpdateWorkspaceNameSession(context.Context, []byte, store.UpdateWorkspaceNameCommand) (store.Workspace, error)
}

type IdentityFlow interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string, time.Time) (identity.Authentication, error)
}

type IdentityHandler struct {
	store  IdentityStore
	flow   IdentityFlow
	origin string
	logger *slog.Logger
}

func NewIdentityHandler(database IdentityStore, flow IdentityFlow, origin string, logger *slog.Logger) (*IdentityHandler, error) {
	u, err := url.Parse(origin)
	if database == nil || flow == nil || logger == nil || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("invalid identity HTTP dependencies")
	}
	return &IdentityHandler{store: database, flow: flow, origin: origin, logger: logger}, nil
}

func (h *IdentityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This API uses only the opaque cookie. Competing credentials are ambiguous.
	if len(r.Header.Values("Authorization")) != 0 {
		WriteError(w, r, http.StatusUnauthorized, "authentication_required")
		return
	}
	switch r.URL.Path {
	case "/api/v1/auth/start":
		h.start(w, r)
		return
	case "/api/v1/auth/callback":
		h.callback(w, r)
		return
	}
	token, hash, err := credential(r, sessionCookie, true)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	session, err := h.store.ResolveSession(r.Context(), hash)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !h.sameOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfProof(token))) != 1 || len(r.Header.Values("X-CSRF-Token")) != 1 {
			WriteError(w, r, http.StatusForbidden, "csrf_rejected")
			return
		}
	}
	if h.listingDraft(w, r, hash) {
		return
	}
	switch r.URL.Path {
	case "/api/v1/session":
		if !method(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, sessionResponse(session, csrfProof(token)))
	case "/api/v1/session/logout", "/api/v1/session/revoke-all":
		if !method(w, r, http.MethodPost) {
			return
		}
		if err := decodeCommand(r, &struct{}{}); err != nil {
			h.failure(w, r, err)
			return
		}
		if err := h.store.RevokeSession(r.Context(), hash, r.URL.Path == "/api/v1/session/revoke-all", RequestID(r.Context()), RequestID(r.Context())); err != nil {
			h.failure(w, r, err)
			return
		}
		clearCookie(w, sessionCookie)
		w.WriteHeader(http.StatusNoContent)
	default:
		const prefix = "/api/v1/workspaces/"
		if !strings.HasPrefix(r.URL.Path, prefix) || strings.Contains(strings.TrimPrefix(r.URL.Path, prefix), "/") || len(strings.TrimPrefix(r.URL.Path, prefix)) != 36 {
			WriteError(w, r, http.StatusNotFound, "not_found")
			return
		}
		workspaceID := strings.TrimPrefix(r.URL.Path, prefix)
		var workspace store.Workspace
		switch r.Method {
		case http.MethodGet:
			workspace, err = h.store.ReadWorkspaceSession(r.Context(), hash, workspaceID)
		case http.MethodPatch:
			var body struct {
				Name            string `json:"name"`
				ExpectedVersion int64  `json:"expected_version"`
			}
			if err = decodeCommand(r, &body, "name", "expected_version"); err != nil {
				h.failure(w, r, err)
				return
			}
			workspace, err = h.store.UpdateWorkspaceNameSession(r.Context(), hash, store.UpdateWorkspaceNameCommand{WorkspaceID: workspaceID, Name: body.Name, ExpectedVersion: body.ExpectedVersion, RequestID: RequestID(r.Context()), CorrelationID: RequestID(r.Context())})
		default:
			w.Header().Set("Allow", "GET, PATCH")
			WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if err != nil {
			h.failure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, workspace)
	}
}

func (h *IdentityHandler) start(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !h.sameOrigin(r) {
		WriteError(w, r, http.StatusForbidden, "csrf_rejected")
		return
	}
	if err := decodeCommand(r, &struct{}{}); err != nil {
		h.failure(w, r, err)
		return
	}
	values := make([]string, 4)
	for i := range values {
		value, err := identity.NewToken()
		if err != nil {
			h.failure(w, r, identity.ErrUnavailable)
			return
		}
		values[i] = value
	}
	state, binding, nonce, verifier := values[0], values[1], values[2], values[3]
	stateHash, err := identity.HashToken(state)
	if err != nil {
		h.failure(w, r, identity.ErrUnavailable)
		return
	}
	bindingHash, err := identity.HashToken(binding)
	if err != nil {
		h.failure(w, r, identity.ErrUnavailable)
		return
	}
	if err := h.store.CreateLogin(r.Context(), store.LoginTransaction{StateHash: stateHash, BindingHash: bindingHash, Nonce: nonce, PKCEVerifier: verifier, RequestID: RequestID(r.Context()), CorrelationID: RequestID(r.Context())}); err != nil {
		h.failure(w, r, err)
		return
	}
	setCookie(w, loginCookie, binding, 300)
	writeJSON(w, http.StatusOK, map[string]string{"authorization_url": h.flow.AuthorizationURL(state, nonce, verifier)})
}

func (h *IdentityHandler) callback(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["state"]) != 1 || len(query["code"]) != 1 || query.Get("error") != "" || len(query.Get("code")) < 1 || len(query.Get("code")) > 2048 {
		h.failure(w, r, identity.ErrUnauthenticated)
		return
	}
	// Never accept return URLs, response_mode changes or duplicated protocol fields.
	for key, values := range query {
		if len(values) != 1 || (key != "state" && key != "code" && key != "iss") {
			h.failure(w, r, identity.ErrUnauthenticated)
			return
		}
	}
	_, bindingHash, err := credential(r, loginCookie, true)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	stateHash, err := identity.HashToken(query.Get("state"))
	if err != nil {
		h.failure(w, r, identity.ErrUnauthenticated)
		return
	}
	_, previousHash, err := credential(r, sessionCookie, false)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	login, err := h.store.ConsumeLogin(r.Context(), stateHash, bindingHash)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	clearCookie(w, loginCookie)
	facts, err := h.flow.Exchange(r.Context(), query.Get("code"), login.PKCEVerifier, login.Nonce, login.StartedAt)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if issuer := query.Get("iss"); issuer != "" && issuer != facts.Issuer {
		h.failure(w, r, identity.ErrUnauthenticated)
		return
	}
	token, err := identity.NewToken()
	if err != nil {
		h.failure(w, r, identity.ErrUnavailable)
		return
	}
	hash, err := identity.HashToken(token)
	if err != nil {
		h.failure(w, r, identity.ErrUnavailable)
		return
	}
	session, err := h.store.CompleteLogin(r.Context(), facts, hash, previousHash, login.StartedAt, RequestID(r.Context()), login.CorrelationID)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	// A browser-session cookie; authoritative idle/absolute deadlines live in PG.
	setCookie(w, sessionCookie, token, 0)
	writeJSON(w, http.StatusOK, sessionResponse(session, csrfProof(token)))
}

func credential(r *http.Request, name string, required bool) (string, []byte, error) {
	var value string
	count := 0
	// Count raw named entries too: net/http silently drops some malformed
	// cookies, which otherwise hides an ambiguous duplicate credential.
	for _, header := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			key, raw, found := strings.Cut(strings.TrimSpace(part), "=")
			if strings.TrimSpace(key) == name {
				count++
				if !found || raw != strings.TrimSpace(raw) {
					return "", nil, identity.ErrUnauthenticated
				}
				value = raw
			}
		}
	}
	if count == 0 && !required {
		return "", nil, nil
	}
	if count != 1 {
		return "", nil, identity.ErrUnauthenticated
	}
	hash, err := identity.HashToken(value)
	if err != nil {
		return "", nil, identity.ErrUnauthenticated
	}
	return value, hash, nil
}

func csrfProof(token string) string {
	hash := sha256.Sum256([]byte("csrf-v1:" + token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func (h *IdentityHandler) sameOrigin(r *http.Request) bool {
	return len(r.Header.Values("Origin")) == 1 && r.Header.Get("Origin") == h.origin
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func clearCookie(w http.ResponseWriter, name string) { setCookie(w, name, "", -1) }

func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
	return false
}

func decodeCommand(r *http.Request, target any, allowedKeys ...string) error {
	if len(r.Header.Values("Content-Type")) != 1 {
		return store.ErrInvalid
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(r.Body)
	if err != nil || !validJSONEncoding(data) {
		return store.ErrInvalid
	}
	// Go's decoder normally accepts duplicate keys and null into structs. Reject
	// both instead of letting different layers interpret ambiguous commands.
	shape := json.NewDecoder(bytes.NewReader(data))
	first, err := shape.Token()
	if err != nil || first != json.Delim('{') {
		return store.ErrInvalid
	}
	seen := map[string]bool{}
	for shape.More() {
		key, err := shape.Token()
		if err != nil {
			return store.ErrInvalid
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return store.ErrInvalid
		}
		allowed := false
		for _, candidate := range allowedKeys {
			if name == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return store.ErrInvalid
		}
		seen[name] = true
		var value json.RawMessage
		if err := shape.Decode(&value); err != nil {
			return store.ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return store.ErrInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return store.ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return store.ErrInvalid
	}
	return nil
}

func sessionResponse(s store.Session, proof string) any {
	// Explicit projection: provider claims, subject, cookies and security versions
	// are never serialized. This response is same-origin and Cache-Control:no-store.
	return struct {
		PrincipalID       string             `json:"principal_id"`
		UserID            string             `json:"user_id"`
		SessionID         string             `json:"session_id"`
		ActorKind         identity.ActorKind `json:"actor_kind"`
		AuthenticatedAt   time.Time          `json:"authenticated_at"`
		AbsoluteExpiresAt time.Time          `json:"absolute_expires_at"`
		IdleExpiresAt     time.Time          `json:"idle_expires_at"`
		CSRFToken         string             `json:"csrf_token"`
	}{s.Principal.PrincipalID, s.Principal.UserID, s.Principal.SessionID, s.Principal.Kind, s.Principal.Authentication.AuthenticatedAt, s.AbsoluteExpiresAt, s.IdleExpiresAt, proof}
}

func (h *IdentityHandler) failure(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusServiceUnavailable, "dependency_unavailable"
	switch {
	case errors.Is(err, store.ErrUnavailable), errors.Is(err, identity.ErrUnavailable):
		// A rollback/commit dependency fault can be joined to a domain denial.
		// Preserve its operational failure instead of disguising it as bad login.
		status, code = http.StatusServiceUnavailable, "dependency_unavailable"
	case errors.Is(err, identity.ErrUnauthenticated):
		status, code = http.StatusUnauthorized, "authentication_required"
	case errors.Is(err, store.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrConflict):
		status, code = http.StatusConflict, "version_conflict"
	case errors.Is(err, store.ErrInvalid), errors.Is(err, identity.ErrInvalid):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, store.ErrRateLimited):
		status, code = http.StatusTooManyRequests, "rate_limited"
		w.Header().Set("Retry-After", "60")
	}
	// Do not log error strings: dependencies can carry tokens, claims or DB values.
	h.logger.WarnContext(r.Context(), "identity_request_failed", "code", code, "request_id", RequestID(r.Context()))
	WriteError(w, r, status, code)
}
