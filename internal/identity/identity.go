// Package identity is the application authentication boundary. Provider facts
// are deliberately separate from workspace authorization and seller authority.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var (
	ErrUnauthenticated = errors.New("identity: unauthenticated")
	ErrUnavailable     = errors.New("identity: dependency unavailable")
	ErrInvalid         = errors.New("identity: invalid input")
	ErrStepUpRequired  = errors.New("identity: fresh elevated authentication required")
)

type ActorKind string

const (
	Human             ActorKind = "HUMAN"
	Service           ActorKind = "SERVICE"
	SessionCookieName           = "__Host-marketplace-session"
	ExternalClockSkew           = 60 * time.Second
	StepUpFreshness             = 5 * time.Minute
)

type AssuranceLevel string

const (
	AssuranceUnknown AssuranceLevel = "UNKNOWN"
	AssuranceMFA     AssuranceLevel = "MFA"
)

// Assurance describes evidence accepted by an authentication adapter, never a
// caller-supplied role or an inference from email verification.
type Assurance struct {
	Level    AssuranceLevel
	Evidence string
}

type CredentialSource string

const (
	OIDCSession           CredentialSource = "oidc-session"
	PostgreSQLRuntimeRole CredentialSource = "postgres-runtime-role"
)

type Authentication struct {
	Provider        string
	Issuer          string
	Subject         string
	AuthenticatedAt time.Time
	EmailVerified   bool
	Assurance       Assurance
}

type Principal struct {
	PrincipalID      string
	UserID           string
	SessionID        string
	Kind             ActorKind
	CredentialSource CredentialSource
	Authentication   Authentication
}

// OutboxWorkerPrincipal identifies the independently running worker after its
// PostgreSQL runtime role has been validated. This value records attribution; it
// grants no authority and is never accepted from an HTTP request.
func OutboxWorkerPrincipal() Principal {
	return Principal{PrincipalID: "40000000-0000-4000-8000-000000000001", Kind: Service, CredentialSource: PostgreSQLRuntimeRole}
}

// Flow is implemented by a trusted identity adapter. Its arguments are generated
// and retained by the application login transaction, not taken as provider facts.
type Flow interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(ctx context.Context, code, verifier, nonce string, startedAt time.Time) (Authentication, error)
}

type Permission string

const (
	WorkspaceRead      Permission = "workspace.read"
	WorkspaceUpdate    Permission = "workspace.update"
	ListingReadPrivate Permission = "listing.read_private"
	ListingCreate      Permission = "listing.create"
	ListingUpdate      Permission = "listing.update"
)

type RoleTemplate string

const (
	Reader RoleTemplate = "READER"
	Editor RoleTemplate = "EDITOR"
)

// Permissions returns fresh explicit grants. A template does not create a
// membership or authorize a request; persisted workspace grants remain authority.
func (r RoleTemplate) Permissions() ([]Permission, error) {
	switch r {
	case Reader:
		return []Permission{WorkspaceRead}, nil
	case Editor:
		return []Permission{WorkspaceRead, WorkspaceUpdate}, nil
	default:
		return nil, ErrInvalid
	}
}

// RequireStepUp is a policy mechanism for future elevated operations. No current
// adapter asserts MFA without an independently proven provider interpretation.
func RequireStepUp(p Principal, now time.Time) error {
	a := p.Authentication
	if p.Kind != Human || p.PrincipalID == "" || p.UserID == "" || p.SessionID == "" ||
		a.Assurance.Level != AssuranceMFA || a.Assurance.Evidence == "" || a.AuthenticatedAt.IsZero() ||
		a.AuthenticatedAt.After(now.Add(ExternalClockSkew)) || !now.Before(a.AuthenticatedAt.Add(StepUpFreshness)) {
		return ErrStepUpRequired
	}
	return nil
}

// NewToken creates a 256-bit opaque credential. Never log the returned value.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken accepts only the single canonical encoding emitted by NewToken.
// Store this digest, never the browser's credential.
func HashToken(token string) ([]byte, error) {
	if len(token) != 43 || strings.TrimSpace(token) != token {
		return nil, ErrUnauthenticated
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(b) != 32 {
		return nil, ErrUnauthenticated
	}
	digest := sha256.Sum256(b)
	return digest[:], nil
}
