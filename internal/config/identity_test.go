package config

import (
	"strings"
	"testing"
)

func identityConfigValues() map[string]string {
	return map[string]string{
		"DATABASE_URL":        "postgres://synthetic@localhost/foundation?sslmode=disable",
		"HTTP_ADDR":           "127.0.0.1:8080",
		"AUTHENTICATION_MODE": "oidc",
		"PUBLIC_ORIGIN":       "https://marketplace.example",
		"OIDC_PROVIDER":       "synthetic",
		"OIDC_ISSUER":         "https://issuer.example/",
		"OIDC_CLIENT_ID":      "synthetic-client",
		"OIDC_CLIENT_SECRET":  "synthetic-not-a-real-secret",
	}
}

func TestIdentityConfigurationIsExplicitAndFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing mode", "AUTHENTICATION_MODE", ""},
		{"unknown mode", "AUTHENTICATION_MODE", "development"},
		{"disabled with OIDC", "AUTHENTICATION_MODE", "disabled"},
		{"missing secret", "OIDC_CLIENT_SECRET", ""},
		{"missing provider", "OIDC_PROVIDER", ""},
		{"insecure issuer", "OIDC_ISSUER", "http://issuer.example/"},
		{"issuer credentials", "OIDC_ISSUER", "https://secret-canary@issuer.example/"},
		{"origin path", "PUBLIC_ORIGIN", "https://marketplace.example/login"},
		{"origin query", "PUBLIC_ORIGIN", "https://marketplace.example?secret-canary"},
		{"origin credentials", "PUBLIC_ORIGIN", "https://secret-canary@marketplace.example"},
		{"insecure origin", "PUBLIC_ORIGIN", "http://marketplace.example"},
		{"mixedcase origin", "PUBLIC_ORIGIN", "https://MarketPlace.example"},
		{"wildcard endpoint", "OIDC_ENDPOINT_ORIGINS", "https://*.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := identityConfigValues()
			values[tc.key] = tc.value
			_, err := Load(mapLookup(values), API)
			if err == nil || strings.Contains(err.Error(), "secret-canary") {
				t.Fatalf("expected safe rejection, got %v", err)
			}
		})
	}
	values := identityConfigValues()
	cfg, err := Load(mapLookup(values), API)
	if err != nil || cfg.Identity.RedirectURI != values["PUBLIC_ORIGIN"]+"/api/v1/auth/callback" {
		t.Fatalf("valid exact callback configuration: %v", err)
	}
	values["AUTHENTICATION_MODE"] = "unrecognized"
	if _, err := Load(mapLookup(values), Worker); err != nil {
		t.Fatalf("worker must remain independent from browser credentials: %v", err)
	}
}
