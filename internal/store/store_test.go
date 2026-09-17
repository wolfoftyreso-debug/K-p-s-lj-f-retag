package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIdentifiers(t *testing.T) {
	for _, value := range []string{"", "not-an-id", "00000000-0000-4000-8000-00000000000z", "00000000-0000-4000-8000-00000000000A"} {
		if validID(value) {
			t.Errorf("accepted %q", value)
		}
	}
	a, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	if !validID(a) || !validID(b) || a == b || a[14] != '4' {
		t.Fatal("invalid/randomly repeated generated UUID")
	}
}
func TestNames(t *testing.T) {
	for _, name := range []string{"", " ", " Leading", "Trailing ", "bad\nvalue", strings.Repeat("a", 121), string([]byte{0xff})} {
		if validName(name) {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"Acme", "Björk", strings.Repeat("ö", 120)} {
		if !validName(name) {
			t.Errorf("rejected %q", name)
		}
	}
}
func TestFaultDoesNotExposeDatabaseDetails(t *testing.T) {
	err := fault("audit_append", &pgconn.PgError{Code: "23514", Message: "secret payload", Detail: "private row"})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe error %v", err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		err := fault("connect", fmt.Errorf("secret connection detail: %w", cause))
		if !errors.Is(err, cause) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe cancellation: %v", err)
		}
	}
}
func TestWorkerConfiguration(t *testing.T) {
	s := &Store{role: "worker"}
	for _, o := range []WorkerOptions{{}, {LeaseDuration: time.Millisecond, RetryBase: time.Second, MaxAttempts: 5}, {LeaseDuration: time.Second, RetryBase: time.Second, MaxAttempts: 21}} {
		if _, err := NewProcessor(s, o); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid config: %v", err)
		}
	}
	if _, err := NewProcessor(s, DefaultWorkerOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProcessor(&Store{role: "api"}, DefaultWorkerOptions()); !errors.Is(err, ErrUnsafeRole) {
		t.Fatal(err)
	}
}
func TestEnvelopeValidation(t *testing.T) {
	d := delivery{EventType: "WorkspaceNameChanged", SchemaVersion: 1, Version: 2, Payload: []byte(`{}`)}
	if !validDelivery(d) {
		t.Fatal("rejected supported envelope")
	}
	for _, payload := range []string{`null`, `[]`, `{"name":"private"}`, `invalid`} {
		d.Payload = []byte(payload)
		if validDelivery(d) {
			t.Fatalf("accepted %s", payload)
		}
	}
	d.Payload = []byte(`{}`)
	d.SchemaVersion = 2
	if validDelivery(d) {
		t.Fatal("accepted unsupported schema")
	}
}
