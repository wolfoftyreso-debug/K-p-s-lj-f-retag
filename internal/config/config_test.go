package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadValid(t *testing.T) {
	t.Parallel()
	for _, process := range []Process{API, Worker} {
		t.Run(string(process), func(t *testing.T) {
			cfg, err := Load(mapLookup(map[string]string{"DATABASE_URL": "postgres://synthetic@localhost/foundation?sslmode=disable", "HTTP_ADDR": "127.0.0.1:8080"}), process)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ShutdownTimeout != 15*time.Second || cfg.MaxRequestBody != 16384 {
				t.Fatalf("unexpected defaults: %s %d", cfg.ShutdownTimeout, cfg.MaxRequestBody)
			}
		})
	}
}

func TestLoadRejectsMalformedConfigurationWithoutEchoingValues(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, key, value string }{
		{"missing database", "DATABASE_URL", ""},
		{"invalid URI", "DATABASE_URL", "postgres://secret-canary:%zz@localhost/database"},
		{"wrong scheme", "DATABASE_URL", "https://secret-canary/database"},
		{"missing database path", "DATABASE_URL", "postgres://localhost"},
		{"database fragment", "DATABASE_URL", "postgres://localhost/test#secret-canary"},
		{"missing username", "DATABASE_URL", "postgres://localhost/test?sslmode=disable"},
		{"missing TLS policy", "DATABASE_URL", "postgres://synthetic@localhost/test"},
		{"ambiguous TLS policy", "DATABASE_URL", "postgres://synthetic@localhost/test?sslmode=disable&sslmode=prefer"},
		{"TLS downgrade", "DATABASE_URL", "postgres://synthetic@localhost/test?sslmode=prefer"},
		{"remote plaintext", "DATABASE_URL", "postgres://synthetic@database.example/test?sslmode=disable"},
		{"query host override", "DATABASE_URL", "postgres://synthetic@localhost/test?sslmode=disable&host=database.example"},
		{"query identity override", "DATABASE_URL", "postgres://synthetic@localhost/test?sslmode=disable&user=admin"},
		{"service override", "DATABASE_URL", "postgres://synthetic@localhost/test?sslmode=disable&service=unreviewed"},
		{"missing address", "HTTP_ADDR", ""},
		{"non-numeric port", "HTTP_ADDR", "localhost:secret-canary"},
		{"negative port", "HTTP_ADDR", "localhost:-1"},
		{"zero port", "HTTP_ADDR", "localhost:0"},
		{"port overflow", "HTTP_ADDR", "localhost:65536"},
		{"invalid duration", "SHUTDOWN_TIMEOUT", "secret-canary"},
		{"negative duration", "WORKER_POLL_INTERVAL", "-1s"},
		{"excessive duration", "REQUEST_TIMEOUT", "1h"},
		{"readiness exceeds request", "REQUEST_TIMEOUT", "500ms"},
		{"invalid body limit", "MAX_REQUEST_BODY_BYTES", "secret-canary"},
		{"negative body limit", "MAX_REQUEST_BODY_BYTES", "-1"},
		{"unbounded body limit", "MAX_REQUEST_BODY_BYTES", "9999999999"},
		{"unbounded attempts", "WORKER_MAX_ATTEMPTS", "101"},
		{"zero attempts", "WORKER_MAX_ATTEMPTS", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://synthetic@localhost/foundation?sslmode=disable", "HTTP_ADDR": ":8080"}
			values[tc.key] = tc.value
			_, err := Load(mapLookup(values), API)
			if err == nil || strings.Contains(err.Error(), "secret-canary") {
				t.Fatalf("expected safe configuration error, got %v", err)
			}
		})
	}
}

func TestLoadAllowsVerifiedRemoteTLSAndExplicitLoopback(t *testing.T) {
	for _, dsn := range []string{
		"postgres://synthetic@database.example/foundation?sslmode=verify-full",
		"postgres://synthetic@127.0.0.1/foundation?sslmode=disable",
		"postgres://synthetic@[::1]/foundation?sslmode=disable",
	} {
		if _, err := Load(mapLookup(map[string]string{"DATABASE_URL": dsn, "HTTP_ADDR": ":8080"}), API); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadRejectsInvalidSourceAndProcess(t *testing.T) {
	if _, err := Load(nil, API); err == nil {
		t.Fatal("nil lookup accepted")
	}
	if _, err := Load(mapLookup(nil), Process("unknown")); err == nil {
		t.Fatal("unknown process accepted")
	}
}

func mapLookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
