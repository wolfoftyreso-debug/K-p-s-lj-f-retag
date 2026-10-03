package identity

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

const maxProviderResponse = 256 * 1024

var providerName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type Config struct {
	Provider     string
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	// Additional exact HTTPS origins permitted in discovery metadata. The issuer
	// origin is always included. This is trusted deployment configuration only.
	EndpointOrigins []string
}

func (c Config) Validate() error {
	if !providerName.MatchString(c.Provider) || !boundedText(c.ClientID, 256) || !boundedText(c.ClientSecret, 4096) {
		return fmt.Errorf("%w: provider or client configuration", ErrInvalid)
	}
	if _, err := secureURL(c.Issuer); err != nil {
		return fmt.Errorf("%w: issuer", ErrInvalid)
	}
	if _, err := secureURL(c.RedirectURI); err != nil {
		return fmt.Errorf("%w: callback", ErrInvalid)
	}
	if len(c.EndpointOrigins) > 4 {
		return fmt.Errorf("%w: endpoint origins", ErrInvalid)
	}
	for _, raw := range c.EndpointOrigins {
		u, err := secureURL(raw)
		if err != nil || u.Path != "" || u.String() != origin(u) {
			return fmt.Errorf("%w: endpoint origin", ErrInvalid)
		}
	}
	return nil
}

// OIDC uses authorization code + PKCE and never stores provider refresh tokens.
// New login fetches the configured provider's current keys each time. No stale
// key cache or authentication fallback exists during a provider outage.
type OIDC struct {
	config  Config
	oauth   oauth2.Config
	client  *http.Client
	jwksURL string
	now     func() time.Time
}

func NewOIDC(ctx context.Context, c Config) (*OIDC, error) {
	transport := &http.Transport{
		DialContext:            (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    3 * time.Second,
		ResponseHeaderTimeout:  3 * time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		MaxResponseHeaderBytes: 16 * 1024,
	}
	adapter, err := newOIDC(ctx, c, transport)
	if err != nil {
		transport.CloseIdleConnections()
	}
	return adapter, err
}

// newOIDC keeps synthetic transport injection inside package tests. There is no
// production TLS bypass, development identity or externally configurable clock.
func newOIDC(ctx context.Context, c Config, transport http.RoundTripper) (*OIDC, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	discoveryURL := strings.TrimSuffix(c.Issuer, "/") + "/.well-known/openid-configuration"
	guard := &providerTransport{base: transport, endpoints: map[string]string{discoveryURL: http.MethodGet}}
	client := &http.Client{Transport: guard, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return ErrUnavailable }}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if err != nil {
		return nil, safeDependencyError(ctx)
	}
	var metadata struct {
		Issuer     string   `json:"issuer"`
		JWKSURL    string   `json:"jwks_uri"`
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if provider.Claims(&metadata) != nil || metadata.Issuer != c.Issuer || !slices.Contains(metadata.Algorithms, "RS256") {
		return nil, ErrUnavailable
	}
	issuerURL, _ := secureURL(c.Issuer) // Validate already checked this value.
	origins := append([]string{origin(issuerURL)}, c.EndpointOrigins...)
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKSURL} {
		u, err := secureURL(raw)
		if err != nil || !slices.Contains(origins, origin(u)) {
			return nil, ErrUnavailable
		}
	}
	if endpoint.TokenURL == discoveryURL || metadata.JWKSURL == discoveryURL || endpoint.TokenURL == metadata.JWKSURL {
		return nil, ErrUnavailable
	}
	guard.endpoints[endpoint.TokenURL] = http.MethodPost
	guard.endpoints[metadata.JWKSURL] = http.MethodGet
	// Explicit Basic auth avoids library probing/replaying an authorization code
	// using a different client-auth method after a failed exchange.
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	return &OIDC{config: c, client: client, jwksURL: metadata.JWKSURL, now: time.Now,
		oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.RedirectURI,
			Endpoint: endpoint, Scopes: []string{oidc.ScopeOpenID, "email"}}}, nil
}

func (o *OIDC) Close() { o.client.CloseIdleConnections() }

func (o *OIDC) AuthorizationURL(state, nonce, verifier string) string {
	for _, token := range []string{state, nonce, verifier} {
		if _, err := HashToken(token); err != nil {
			return ""
		}
	}
	return o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("max_age", "0"), oauth2.SetAuthURLParam("prompt", "login"))
}

func (o *OIDC) Exchange(ctx context.Context, code, verifier, nonce string, startedAt time.Time) (Authentication, error) {
	if err := ctx.Err(); err != nil {
		return Authentication{}, err
	}
	if !boundedText(code, 4096) || startedAt.IsZero() || startedAt.After(o.now().Add(ExternalClockSkew)) {
		return Authentication{}, ErrUnauthenticated
	}
	for _, token := range []string{nonce, verifier} {
		if _, err := HashToken(token); err != nil {
			return Authentication{}, ErrUnauthenticated
		}
	}
	token, err := o.oauth.Exchange(oidc.ClientContext(ctx, o.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		if ctx.Err() != nil {
			return Authentication{}, ctx.Err()
		}
		var failure *oauth2.RetrieveError
		if errors.As(err, &failure) && failure.ErrorCode == "invalid_grant" {
			return Authentication{}, ErrUnauthenticated
		}
		return Authentication{}, ErrUnavailable
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || !validJWTEnvelope(raw) {
		return Authentication{}, ErrUnauthenticated
	}
	parts := strings.Split(raw, ".")
	header, _ := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	var signatureHeader struct {
		KeyID string `json:"kid"`
	}
	if json.Unmarshal(header, &signatureHeader) != nil || len(signatureHeader.KeyID) > 256 {
		return Authentication{}, ErrUnauthenticated
	}
	keys, err := o.keys(ctx, signatureHeader.KeyID)
	if err != nil {
		return Authentication{}, err
	}
	verified, err := oidc.NewVerifier(o.config.Issuer, &oidc.StaticKeySet{PublicKeys: keys},
		&oidc.Config{ClientID: o.config.ClientID, SupportedSigningAlgs: []string{oidc.RS256}, Now: o.now}).Verify(ctx, raw)
	if err != nil {
		return Authentication{}, ErrUnauthenticated
	}
	if verified.AccessTokenHash != "" && verified.VerifyAccessToken(token.AccessToken) != nil {
		return Authentication{}, ErrUnauthenticated
	}
	var payload json.RawMessage
	if verified.Claims(&payload) != nil || uniqueJSON(payload) != nil || !caseSensitiveMembers(payload,
		"iss", "sub", "aud", "exp", "iat", "nbf", "nonce", "auth_time", "email_verified", "azp", "at_hash") {
		return Authentication{}, ErrUnauthenticated
	}
	var claims struct {
		AuthorizedParty string `json:"azp"`
		AuthTime        *int64 `json:"auth_time"`
		NotBefore       *int64 `json:"nbf"`
		EmailVerified   bool   `json:"email_verified"`
	}
	if verified.Claims(&claims) != nil || !claims.EmailVerified || claims.AuthTime == nil || *claims.AuthTime <= 0 ||
		verified.Issuer != o.config.Issuer || !boundedText(verified.Subject, 255) ||
		subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(nonce)) != 1 ||
		(claims.AuthorizedParty != "" && claims.AuthorizedParty != o.config.ClientID) ||
		(len(verified.Audience) != 1 && claims.AuthorizedParty != o.config.ClientID) {
		return Authentication{}, ErrUnauthenticated
	}
	now := o.now()
	authenticatedAt := time.Unix(*claims.AuthTime, 0).UTC()
	// Override the library's broader nbf grace with this application's documented
	// maximum. Expired credentials receive no extra expiry grace.
	if !verified.Expiry.After(now) || verified.IssuedAt.IsZero() || verified.IssuedAt.After(now.Add(ExternalClockSkew)) ||
		verified.IssuedAt.Before(startedAt.Add(-ExternalClockSkew)) ||
		authenticatedAt.After(now.Add(ExternalClockSkew)) || authenticatedAt.Before(startedAt.Add(-ExternalClockSkew)) ||
		authenticatedAt.After(verified.IssuedAt.Add(ExternalClockSkew)) ||
		(claims.NotBefore != nil && time.Unix(*claims.NotBefore, 0).After(now.Add(ExternalClockSkew))) {
		return Authentication{}, ErrUnauthenticated
	}
	return Authentication{Provider: o.config.Provider, Issuer: verified.Issuer, Subject: verified.Subject,
		AuthenticatedAt: authenticatedAt, EmailVerified: true, Assurance: Assurance{Level: AssuranceUnknown}}, nil
}

func (o *OIDC) keys(ctx context.Context, keyID string) ([]crypto.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, o.jwksURL, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	response, err := o.client.Do(request)
	if err != nil {
		return nil, safeDependencyError(ctx)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	var set jose.JSONWebKeySet
	if json.NewDecoder(response.Body).Decode(&set) != nil || len(set.Keys) == 0 || len(set.Keys) > 16 {
		return nil, ErrUnavailable
	}
	keys := make([]crypto.PublicKey, 0, len(set.Keys))
	validKeys := 0
	for _, key := range set.Keys {
		publicKey, ok := key.Key.(*rsa.PublicKey)
		if !ok || !key.IsPublic() || !key.Valid() || (key.Use != "" && key.Use != "sig") ||
			(key.Algorithm != "" && key.Algorithm != "RS256") || publicKey.N.BitLen() < 2048 || publicKey.N.BitLen() > 8192 {
			continue
		}
		validKeys++
		if keyID != "" && key.KeyID != keyID {
			continue
		}
		keys = append(keys, publicKey)
	}
	if len(keys) == 0 {
		if validKeys > 0 {
			return nil, ErrUnauthenticated
		}
		return nil, ErrUnavailable
	}
	return keys, nil
}

func validJWTEnvelope(raw string) bool {
	if len(raw) > 16*1024 {
		return false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[2] == "" {
		return false
	}
	header, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || uniqueJSON(header) != nil || !caseSensitiveMembers(header, "alg", "kid", "jku", "jwk", "x5u", "crit") {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(header, &fields) != nil {
		return false
	}
	for _, forbidden := range []string{"jku", "x5u", "jwk"} {
		if _, exists := fields[forbidden]; exists {
			return false
		}
	}
	return true
}

// OIDC and JOSE member names are case-sensitive. Go's struct decoder accepts
// case-folded aliases; reject those before any decoded security fact can leave
// this boundary, including an alias alongside the correctly spelled member.
func caseSensitiveMembers(data []byte, protected ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return false
	}
	for name := range fields {
		for _, exact := range protected {
			if name != exact && strings.EqualFold(name, exact) {
				return false
			}
		}
	}
	return true
}

type providerTransport struct {
	base      http.RoundTripper
	endpoints map[string]string // Immutable before the adapter is exposed.
}

func (t *providerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if method, ok := t.endpoints[request.URL.String()]; !ok || request.Method != method {
		return nil, ErrUnavailable
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxProviderResponse+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(body) > maxProviderResponse || uniqueJSON(body) != nil {
		return nil, ErrUnavailable
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func (t *providerTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func secureURL(raw string) (*url.URL, error) {
	if !boundedText(raw, 2048) {
		return nil, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return nil, ErrInvalid
	}
	if !validHostname(u.Hostname()) {
		return nil, ErrInvalid
	}
	return u, nil
}

func validHostname(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func origin(u *url.URL) string { return u.Scheme + "://" + u.Host }

func boundedText(text string, limit int) bool {
	return len(text) > 0 && len(text) <= limit && strings.TrimSpace(text) == text &&
		!strings.ContainsFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

func safeDependencyError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrUnavailable
}

// uniqueJSON rejects duplicate members at every depth and trailing documents.
// Ambiguous provider or signed-claim representations must not establish identity.
func uniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := readJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func readJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrInvalid
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			member, err := decoder.Token()
			name, ok := member.(string)
			if err != nil || !ok {
				return ErrInvalid
			}
			if _, exists := seen[name]; exists {
				return ErrInvalid
			}
			seen[name] = struct{}{}
			if err := readJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := readJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = decoder.Token()
	return err
}
