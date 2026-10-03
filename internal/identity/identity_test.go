package identity

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOpaqueToken(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	ha, err := HashToken(a)
	if err != nil || len(ha) != 32 || a == b {
		t.Fatal("credential generation or hash invalid")
	}
	haAgain, err := HashToken(a)
	if err != nil || !bytes.Equal(ha, haAgain) {
		t.Fatal("digest is not deterministic")
	}
	for _, invalid := range []string{"", a + "=", " " + a, strings.Repeat("!", 43), strings.Repeat("A", 42) + "B", a + b} {
		if _, err := HashToken(invalid); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("accepted malformed credential")
		}
	}
}

func TestPermissionTemplatesAreScopedDescriptions(t *testing.T) {
	read, err := Reader.Permissions()
	if err != nil || len(read) != 1 || read[0] != WorkspaceRead {
		t.Fatal("reader template")
	}
	edit, err := Editor.Permissions()
	if err != nil || len(edit) != 2 || edit[0] != WorkspaceRead || edit[1] != WorkspaceUpdate {
		t.Fatal("editor template")
	}
	edit[0] = "forged"
	unchanged, _ := Editor.Permissions()
	if unchanged[0] != WorkspaceRead {
		t.Fatal("template uses mutable global state")
	}
	if _, err := RoleTemplate("OWNER").Permissions(); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown role accepted")
	}
}

func TestWorkerPrincipalCannotBeAHumanSession(t *testing.T) {
	principal := OutboxWorkerPrincipal()
	if principal.Kind != Service || principal.PrincipalID == "" || principal.CredentialSource != "postgres-runtime-role" ||
		principal.UserID != "" || principal.SessionID != "" || principal.Authentication != (Authentication{}) {
		t.Fatal("worker attribution conflates a service and a human")
	}
	if !errors.Is(RequireStepUp(principal, time.Now()), ErrStepUpRequired) {
		t.Fatal("service established human assurance")
	}
}

func TestStepUpRequiresHumanFreshProvenAssurance(t *testing.T) {
	now := time.Now().UTC()
	valid := Principal{Kind: Human, PrincipalID: "principal", UserID: "user", SessionID: "session",
		Authentication: Authentication{AuthenticatedAt: now, Assurance: Assurance{Level: AssuranceMFA, Evidence: "synthetic-reviewed-evidence"}}}
	if err := RequireStepUp(valid, now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Principal){
		"machine":               func(p *Principal) { p.Kind = Service },
		"missing principal":     func(p *Principal) { p.PrincipalID = "" },
		"missing user":          func(p *Principal) { p.UserID = "" },
		"missing session":       func(p *Principal) { p.SessionID = "" },
		"unknown assurance":     func(p *Principal) { p.Authentication.Assurance.Level = AssuranceUnknown },
		"unsupported assurance": func(p *Principal) { p.Authentication.Assurance.Level = "ADMIN" },
		"missing evidence":      func(p *Principal) { p.Authentication.Assurance.Evidence = "" },
		"missing time":          func(p *Principal) { p.Authentication.AuthenticatedAt = time.Time{} },
		"expired freshness":     func(p *Principal) { p.Authentication.AuthenticatedAt = now.Add(-StepUpFreshness) },
		"future clock":          func(p *Principal) { p.Authentication.AuthenticatedAt = now.Add(ExternalClockSkew + time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			p := valid
			mutate(&p)
			if !errors.Is(RequireStepUp(p, now), ErrStepUpRequired) {
				t.Fatal("elevation accepted")
			}
		})
	}
}
