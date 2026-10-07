package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

type protocolFixture struct {
	server        *httptest.Server
	key           *rsa.PrivateKey
	now           time.Time
	nonce         string
	verifier      string
	mu            sync.Mutex
	claims        map[string]any
	rawToken      string
	metadata      map[string]any
	tokenStatus   int
	keyStatus     int
	tokenBody     string
	keyBody       string
	tokenRequests atomic.Int64
	keyRequests   atomic.Int64
	blockToken    bool
	started       chan struct{}
	release       chan struct{}
}

func newProtocolFixture(t *testing.T) *protocolFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	f := &protocolFixture{key: key, now: time.Now().UTC().Truncate(time.Second), nonce: nonce, verifier: verifier,
		tokenStatus: 200, keyStatus: 200, started: make(chan struct{}, 1), release: make(chan struct{})}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(func() { close(f.release); f.server.Close() })
	f.metadata = map[string]any{"issuer": f.server.URL, "authorization_endpoint": f.server.URL + "/authorize",
		"token_endpoint": f.server.URL + "/token", "jwks_uri": f.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}}
	f.reset()
	return f
}

func (f *protocolFixture) reset() {
	f.claims = map[string]any{"iss": f.server.URL, "sub": "synthetic-provider-subject", "aud": "test-client",
		"exp": f.now.Add(10 * time.Minute).Unix(), "iat": f.now.Unix(), "auth_time": f.now.Unix(),
		"nonce": f.nonce, "email_verified": true}
	f.rawToken = ""
	f.tokenBody = ""
	f.keyBody = ""
	f.tokenStatus = 200
	f.keyStatus = 200
}

func (f *protocolFixture) config() Config {
	return Config{Provider: "synthetic", Issuer: f.server.URL, ClientID: "test-client", ClientSecret: "synthetic-not-a-real-credential", RedirectURI: "https://app.example.test/auth/callback"}
}

func (f *protocolFixture) adapter(t *testing.T) *OIDC {
	t.Helper()
	a, err := newOIDC(context.Background(), f.config(), f.server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return f.now }
	t.Cleanup(a.Close)
	return a
}

func sign(t *testing.T, key *rsa.PrivateKey, payload []byte, headers map[jose.HeaderKey]any) string {
	t.Helper()
	options := &jose.SignerOptions{}
	for name, value := range headers {
		options.WithHeader(name, value)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, options)
	if err != nil {
		t.Fatal(err)
	}
	object, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := object.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (f *protocolFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(f.metadata)
	case "/token":
		f.tokenRequests.Add(1)
		if f.blockToken {
			select {
			case f.started <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-f.release:
			}
			return
		}
		user, password, ok := r.BasicAuth()
		if r.Method != http.MethodPost || !ok || user != "test-client" || password != f.config().ClientSecret ||
			r.ParseForm() != nil || r.Form.Get("code_verifier") != f.verifier || r.Form.Get("grant_type") != "authorization_code" ||
			r.Form.Get("redirect_uri") != f.config().RedirectURI {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.WriteHeader(f.tokenStatus)
		if f.tokenBody != "" {
			_, _ = w.Write([]byte(f.tokenBody))
			return
		}
		raw := f.rawToken
		if raw == "" {
			payload, _ := json.Marshal(f.claims)
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, nil)
			if err != nil {
				return
			}
			object, err := signer.Sign(payload)
			if err != nil {
				return
			}
			raw, err = object.CompactSerialize()
			if err != nil {
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access-token", "token_type": "Bearer", "id_token": raw})
	case "/keys":
		f.keyRequests.Add(1)
		w.WriteHeader(f.keyStatus)
		if f.keyBody != "" {
			_, _ = w.Write([]byte(f.keyBody))
			return
		}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, Algorithm: "RS256", Use: "sig", KeyID: "synthetic-key"}}})
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	}
}

func TestOIDCFlowAndCanonicalFacts(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	state, _ := NewToken()
	u, err := url.Parse(a.AuthorizationURL(state, f.nonce, f.verifier))
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	if u.Scheme != "https" || query.Get("state") != state || query.Get("nonce") != f.nonce ||
		query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(f.verifier) || query.Get("code_challenge_method") != "S256" ||
		query.Get("response_type") != "code" || query.Get("max_age") != "0" || query.Get("prompt") != "login" ||
		query.Get("scope") != "openid email" || query.Get("client_secret") != "" {
		t.Fatal("unsafe authorization request")
	}
	if a.AuthorizationURL("forged", f.nonce, f.verifier) != "" {
		t.Fatal("accepted invalid state")
	}
	f.claims["roles"] = []string{"admin"}
	f.claims["amr"] = []string{"mfa"}
	f.claims["email"] = "synthetic@example.test"
	f.claims["organization_id"] = "forged"
	auth, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Subject != "synthetic-provider-subject" || auth.Provider != "synthetic" || auth.Issuer != f.server.URL ||
		!auth.EmailVerified || !auth.AuthenticatedAt.Equal(f.now) || auth.Assurance.Level != AssuranceUnknown || auth.Assurance.Evidence != "" {
		t.Fatal("incorrect canonical authentication or fabricated assurance")
	}
	if f.tokenRequests.Load() != 1 || f.keyRequests.Load() != 1 {
		t.Fatal("unexpected protocol requests")
	}
}

func TestOIDCRejectsInvalidSignedCredentials(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	for name, mutation := range map[string]func(map[string]any){
		"wrong issuer":                   func(c map[string]any) { c["iss"] = "https://attacker.example.test" },
		"wrong audience":                 func(c map[string]any) { c["aud"] = "another-client" },
		"missing audience":               func(c map[string]any) { delete(c, "aud") },
		"multiple audiences without azp": func(c map[string]any) { c["aud"] = []string{"test-client", "another"} },
		"wrong azp":                      func(c map[string]any) { c["azp"] = "another-client" },
		"wrong access token hash":        func(c map[string]any) { c["at_hash"] = "incorrect" },
		"wrong nonce":                    func(c map[string]any) { c["nonce"] = "forged" },
		"missing nonce":                  func(c map[string]any) { delete(c, "nonce") },
		"expired":                        func(c map[string]any) { c["exp"] = f.now.Add(-time.Second).Unix() },
		"expiry boundary":                func(c map[string]any) { c["exp"] = f.now.Unix() },
		"missing expiry":                 func(c map[string]any) { delete(c, "exp") },
		"future issue time":              func(c map[string]any) { c["iat"] = f.now.Add(61 * time.Second).Unix() },
		"missing issue time":             func(c map[string]any) { delete(c, "iat") },
		"old issue time":                 func(c map[string]any) { c["iat"] = f.now.Add(-61 * time.Second).Unix() },
		"future not before":              func(c map[string]any) { c["nbf"] = f.now.Add(61 * time.Second).Unix() },
		"missing auth time":              func(c map[string]any) { delete(c, "auth_time") },
		"null auth time":                 func(c map[string]any) { c["auth_time"] = nil },
		"string auth time":               func(c map[string]any) { c["auth_time"] = "1788888888" },
		"fractional auth time":           func(c map[string]any) { c["auth_time"] = 0.5 },
		"old SSO time":                   func(c map[string]any) { c["auth_time"] = f.now.Add(-61 * time.Second).Unix() },
		"future auth time":               func(c map[string]any) { c["auth_time"] = f.now.Add(61 * time.Second).Unix() },
		"missing subject":                func(c map[string]any) { delete(c, "sub") },
		"unbounded subject":              func(c map[string]any) { c["sub"] = strings.Repeat("s", 256) },
		"control in subject":             func(c map[string]any) { c["sub"] = "synthetic\nsubject" },
		"unverified email":               func(c map[string]any) { c["email_verified"] = false },
		"missing email assertion":        func(c map[string]any) { delete(c, "email_verified") },
		"string verification":            func(c map[string]any) { c["email_verified"] = "true" },
	} {
		t.Run(name, func(t *testing.T) {
			f.reset()
			mutation(f.claims)
			if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("got %v", err)
			}
		})
	}
	f.reset()
	f.claims["aud"] = []string{"test-client", "other"}
	f.claims["azp"] = "test-client"
	if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); err != nil {
		t.Fatal(err)
	}
}

func TestOIDCRejectsMalformedForgedAndAmbiguousTokens(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	payload, _ := json.Marshal(f.claims)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
	duplicated := append([]byte(`{"email_verified":false,`), payload[1:]...)
	hmacSigner, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(strings.Repeat("s", 32))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hmacObject, err := hmacSigner.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	hmacToken, err := hmacObject.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"unapproved HMAC algorithm": hmacToken,
		"unknown key identifier":    sign(t, f.key, payload, map[jose.HeaderKey]any{"kid": "untrusted-key"}),
		"unsigned":                  unsigned,
		"malformed":                 "malformed.token.input",
		"wrong signature":           sign(t, otherKey, payload, nil),
		"duplicate claims":          sign(t, f.key, duplicated, nil),
		"untrusted jwk URL":         sign(t, f.key, payload, map[jose.HeaderKey]any{"jku": "https://attacker.example.test/keys"}),
		"untrusted certificate URL": sign(t, f.key, payload, map[jose.HeaderKey]any{"x5u": "https://attacker.example.test/cert"}),
		"oversized":                 strings.Repeat("x", 16385),
	} {
		t.Run(name, func(t *testing.T) {
			f.rawToken = raw
			if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestOIDCDependencyFailureIsSafeAndNoCodeReplay(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	for name, change := range map[string]func(){
		"token unavailable": func() { f.tokenStatus = 503; f.tokenBody = `{"secret":"must-never-leak"}` },
		"invalid client": func() {
			f.tokenStatus = 401
			f.tokenBody = `{"error":"invalid_client","error_description":"must-never-leak"}`
		},
		"keys unavailable":         func() { f.keyStatus = 503; f.keyBody = `{"secret":"must-never-leak"}` },
		"empty keys":               func() { f.keyBody = `{"keys":[]}` },
		"malformed keys":           func() { f.keyBody = `{"keys":"invalid"}` },
		"duplicate token response": func() { f.tokenBody = `{"id_token":"a","id_token":"b"}` },
		"oversized token response": func() { f.tokenBody = `{"blob":"` + strings.Repeat("a", maxProviderResponse) + `"}` },
	} {
		t.Run(name, func(t *testing.T) {
			f.reset()
			before := f.tokenRequests.Load()
			change()
			_, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now)
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "must-never-leak") {
				t.Fatalf("unsafe error %v", err)
			}
			if f.tokenRequests.Load() != before+1 {
				t.Fatal("authorization code was replayed")
			}
		})
	}
	f.reset()
	f.tokenStatus = 400
	f.tokenBody = `{"error":"invalid_grant","error_description":"must-never-leak"}`
	if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
}

func TestOIDCSecurityMembersAreCaseSensitive(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	for _, name := range []string{"iss", "sub", "aud", "exp", "iat", "nbf", "nonce", "auth_time", "email_verified", "azp", "at_hash"} {
		for _, alongsideCanonical := range []bool{false, true} {
			mode := "alias_only"
			if alongsideCanonical {
				mode = "alias_alongside_canonical"
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				f.reset()
				f.claims["nbf"] = f.now.Unix()
				f.claims["azp"] = "test-client"
				f.claims["at_hash"] = ""
				f.claims[strings.ToUpper(name)] = f.claims[name]
				if !alongsideCanonical {
					delete(f.claims, name)
				}
				payload, err := json.Marshal(f.claims)
				if err != nil {
					t.Fatal(err)
				}
				f.rawToken = sign(t, f.key, payload, nil)
				if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("accepted case-folded security claim: %v", err)
				}
			})
		}
	}
	t.Run("email_verification_alias_overwrites_false", func(t *testing.T) {
		f.reset()
		f.claims["email_verified"] = false
		payload, err := json.Marshal(f.claims)
		if err != nil {
			t.Fatal(err)
		}
		// Preserve deliberate ordering: the alias follows and would overwrite the
		// canonical false value in Go's case-insensitive struct decoding.
		payload = append(payload[:len(payload)-1], []byte(`,"EMAIL_VERIFIED":true}`)...)
		f.rawToken = sign(t, f.key, payload, nil)
		if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("accepted case-folded verification override: %v", err)
		}
	})
	t.Run("unicode_case_folded_issuer", func(t *testing.T) {
		f.reset()
		f.claims["iſſ"] = f.claims["iss"]
		delete(f.claims, "iss")
		payload, err := json.Marshal(f.claims)
		if err != nil {
			t.Fatal(err)
		}
		f.rawToken = sign(t, f.key, payload, nil)
		if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("accepted Unicode case-folded issuer: %v", err)
		}
	})
	f.reset()
	payload, err := json.Marshal(f.claims)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[jose.HeaderKey]any{"ALG": "RS256", "KID": "synthetic-key", "JKU": "https://attacker.example.test/keys",
		"JWK": map[string]string{"kty": "RSA"}, "X5U": "https://attacker.example.test/cert", "CRIT": []string{"unsupported"}} {
		t.Run("header_"+string(name), func(t *testing.T) {
			f.rawToken = sign(t, f.key, payload, map[jose.HeaderKey]any{name: value})
			before := f.keyRequests.Load()
			if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("accepted case-folded JOSE header: %v", err)
			}
			if f.keyRequests.Load() != before {
				t.Fatal("ambiguous header reached key verification")
			}
		})
	}
}

func TestOIDCKeyRotationAndSixtySecondBoundary(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	f.claims["iat"] = f.now.Add(-ExternalClockSkew).Unix()
	f.claims["auth_time"] = f.now.Add(-ExternalClockSkew).Unix()
	f.claims["nbf"] = f.now.Add(ExternalClockSkew).Unix()
	if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); err != nil {
		t.Fatal(err)
	}
	rotated, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.reset()
	f.key = rotated
	if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); err != nil {
		t.Fatal("rotated trusted keys rejected", err)
	}
	if f.keyRequests.Load() != 2 {
		t.Fatal("key rotation used a stale key cache")
	}
	f.keyStatus = http.StatusServiceUnavailable
	f.keyBody = `{}`
	if _, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("used old key during outage")
	}
}

func TestOIDCConfigAndDiscoveryBoundaries(t *testing.T) {
	f := newProtocolFixture(t)
	for name, change := range map[string]func(*Config){
		"missing provider":            func(c *Config) { c.Provider = "" },
		"invalid provider identifier": func(c *Config) { c.Provider = "Bad Provider" },
		"missing client":              func(c *Config) { c.ClientID = "" },
		"missing secret":              func(c *Config) { c.ClientSecret = "" },
		"http issuer":                 func(c *Config) { c.Issuer = "http://issuer.example.test" },
		"issuer query":                func(c *Config) { c.Issuer += "?secret=value" },
		"issuer userinfo":             func(c *Config) { c.Issuer = "https://user:password@issuer.example.test" },
		"callback fragment":           func(c *Config) { c.RedirectURI += "#token" },
		"http callback":               func(c *Config) { c.RedirectURI = "http://app.example.test/callback" },
		"callback query":              func(c *Config) { c.RedirectURI += "?next=evil" },
		"origin with path":            func(c *Config) { c.EndpointOrigins = []string{"https://issuer.example.test/path"} },
		"origin with trailing slash":  func(c *Config) { c.EndpointOrigins = []string{"https://issuer.example.test/"} },
		"wildcard origin":             func(c *Config) { c.EndpointOrigins = []string{"https://*.example.test"} },
		"invalid DNS label":           func(c *Config) { c.EndpointOrigins = []string{"https://-issuer.example.test"} },
	} {
		t.Run(name, func(t *testing.T) {
			c := f.config()
			change(&c)
			if !errors.Is(c.Validate(), ErrInvalid) {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for name, field := range map[string]string{"authorization": "authorization_endpoint", "token": "token_endpoint", "keys": "jwks_uri", "issuer": "issuer"} {
		t.Run("untrusted discovery "+name, func(t *testing.T) {
			previous := f.metadata[field]
			f.metadata[field] = "https://attacker.example.test/path"
			defer func() { f.metadata[field] = previous }()
			if _, err := newOIDC(context.Background(), f.config(), f.server.Client().Transport); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("untrusted metadata accepted: %v", err)
			}
		})
	}
}

func TestOIDCCancellationAndConcurrentExchange(t *testing.T) {
	f := newProtocolFixture(t)
	a := f.adapter(t)
	// Concurrent exchanges exercise the real token/JWKS transport and signature
	// verification. Sign the immutable valid fixture once: generating twelve RSA
	// signatures while serve holds its shared mutex accidentally serializes keys
	// behind expensive fixture work, exceeding the real client's timeout under
	// race instrumentation. Production timeout and exchange concurrency stay intact.
	payload, err := json.Marshal(f.claims)
	if err != nil {
		t.Fatal(err)
	}
	raw := sign(t, f.key, payload, nil)
	f.mu.Lock()
	f.rawToken = raw
	f.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Exchange(ctx, "synthetic-code", f.verifier, f.nonce, f.now); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if f.tokenRequests.Load() != 0 || f.keyRequests.Load() != 0 {
		t.Fatal("already canceled exchange reached the provider")
	}
	const concurrentExchanges = 12
	start := make(chan struct{})
	var group sync.WaitGroup
	for range concurrentExchanges {
		group.Go(func() {
			<-start
			auth, err := a.Exchange(context.Background(), "synthetic-code", f.verifier, f.nonce, f.now)
			if err != nil {
				t.Errorf("parallel exchange failed: %v", err)
			} else if auth.Subject != "synthetic-provider-subject" || auth.Issuer != f.server.URL || !auth.EmailVerified {
				t.Error("parallel exchange did not resolve validated canonical facts")
			}
		})
	}
	close(start)
	group.Wait()
	if f.tokenRequests.Load() != concurrentExchanges || f.keyRequests.Load() != concurrentExchanges {
		t.Fatalf("parallel protocol coverage: token=%d keys=%d want=%d each", f.tokenRequests.Load(), f.keyRequests.Load(), concurrentExchanges)
	}
	f.mu.Lock()
	f.blockToken = true
	f.mu.Unlock()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := a.Exchange(ctx, "synthetic-code", f.verifier, f.nonce, f.now); result <- err }()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("exchange did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation not propagated")
	}
}

func TestProviderTransportRejectsUnlistedRequestsAndRedirects(t *testing.T) {
	var followed atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/followed" {
			followed.Store(true)
		}
		w.Header().Set("Location", "/followed")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	guard := &providerTransport{base: server.Client().Transport, endpoints: map[string]string{server.URL + "/start": http.MethodGet}}
	client := &http.Client{Transport: guard, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrUnavailable }}
	defer client.CloseIdleConnections()
	for _, endpoint := range []string{"/start", "/unlisted", "/start?injected=1"} {
		response, err := client.Get(server.URL + endpoint)
		if response != nil {
			_ = response.Body.Close()
		}
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("request escaped allowlist: %v", err)
		}
	}
	if followed.Load() {
		t.Fatal("followed provider redirect")
	}
}
