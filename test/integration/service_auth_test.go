package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
)

const fixtureSecret = "test-only-machine-client-secret"

type clock struct{ seconds atomic.Int64 }

func newClock() *clock                 { c := new(clock); c.seconds.Store(1792000000); return c }
func (c *clock) now() time.Time        { return time.Unix(c.seconds.Load(), 0) }
func (c *clock) advance(seconds int64) { c.seconds.Add(seconds) }

type stack struct {
	idp                   *fakeIDP
	issuer, serviceB      *httptest.Server
	verifier              *auth.Verifier
	clock                 *clock
	tokenCalls, jwksCalls atomic.Int64
}

func newStack(t *testing.T, roles, scopes []string, clientID string) *stack {
	t.Helper()
	s := &stack{clock: newClock()}
	s.idp = newFakeIDP(t, s.clock, clientID, roles, scopes)
	s.issuer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			s.tokenCalls.Add(1)
		}
		if r.URL.Path == "/.well-known/jwks.json" {
			s.jwksCalls.Add(1)
		}
		s.idp.ServeHTTP(w, r)
	}))
	t.Cleanup(s.issuer.Close)
	var err error
	s.idp.mu.Lock()
	s.idp.issuer = s.issuer.URL
	s.idp.mu.Unlock()
	s.verifier, err = auth.NewVerifier(t.Context(), auth.VerifierConfig{Issuer: s.issuer.URL, Audience: "service-b", JWKSURL: s.issuer.URL + "/.well-known/jwks.json", HTTPClient: s.issuer.Client(), Now: s.clock.now, Leeway: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.verifier.Close)
	resourceHandler, err := api.NewServiceBHandler(s.verifier, "service-a")
	if err != nil {
		t.Fatal(err)
	}
	s.serviceB = httptest.NewTLSServer(resourceHandler)
	t.Cleanup(s.serviceB.Close)
	return s
}

func (s *stack) token(t *testing.T, clientID string, scopes []string) string {
	t.Helper()
	form := url.Values{"grant_type": {"client_credentials"}, "resource": {"service-b"}, "scope": {strings.Join(scopes, " ")}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.issuer.URL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, fixtureSecret)
	response, err := s.issuer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("token status = %d", response.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.AccessToken
}

func requestStatus(t *testing.T, client *http.Client, method, endpoint, token string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

func TestServiceToServicePoliciesAndCaches(t *testing.T) {
	s := newStack(t, []string{"orders:reader", "orders:writer", "orders:admin", "orders:delete"}, []string{"orders:read", "orders:write", "orders:delete"}, "service-a")
	oauth, err := auth.NewOAuthClient(auth.OAuthClientConfig{TokenURL: s.issuer.URL + "/oauth/token", ResourceURL: s.serviceB.URL, Resource: "service-b", ClientID: "service-a", ClientSecret: fixtureSecret, Scopes: []string{"orders:read", "orders:write", "orders:delete"}, HTTPClient: s.issuer.Client(), Now: s.clock.now, RefreshBefore: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oauth.Close)
	serviceA := httptest.NewTLSServer(api.NewServiceAHandler(oauth, s.serviceB.URL))
	defer serviceA.Close()
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/call/orders", 200}, {"POST", "/call/orders", 201}, {"DELETE", "/call/orders/demo-1", 204}} {
		if got := requestStatus(t, serviceA.Client(), tc.method, serviceA.URL+tc.path, ""); got != tc.status {
			t.Fatalf("%s returned %d, want %d", tc.method, got, tc.status)
		}
	}
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := requestStatus(t, oauth.HTTPClient(), "GET", s.serviceB.URL+"/orders", ""); got != 200 {
				t.Errorf("concurrent status = %d", got)
			}
		}()
	}
	wg.Wait()
	if s.tokenCalls.Load() != 1 || s.jwksCalls.Load() != 1 {
		t.Fatalf("cache counts: token=%d JWKS=%d, want 1 each", s.tokenCalls.Load(), s.jwksCalls.Load())
	}
	s.clock.advance(8)
	if got := requestStatus(t, oauth.HTTPClient(), "GET", s.serviceB.URL+"/orders", ""); got != 200 {
		t.Fatal("renewed call failed")
	}
	if s.tokenCalls.Load() != 2 {
		t.Fatal("token was not renewed before expiry")
	}
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		if got := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+path, ""); got != 200 {
			t.Fatalf("public health endpoint returned %d", got)
		}
	}
}

func TestAuthenticatedDenials(t *testing.T) {
	for _, tc := range []struct {
		name, client  string
		roles, scopes []string
		method, path  string
	}{
		{"missing scope", "service-a", []string{"orders:reader"}, []string{"orders:write"}, "GET", "/orders"},
		{"missing role", "service-a", []string{"unrelated"}, []string{"orders:read"}, "GET", "/orders"},
		{"client allowlist", "other-client", []string{"orders:reader"}, []string{"orders:read"}, "GET", "/orders"},
		{"admin is not delete hierarchy", "service-a", []string{"orders:admin"}, []string{"orders:delete"}, "DELETE", "/orders/demo-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t, tc.roles, tc.scopes, tc.client)
			token := s.token(t, tc.client, tc.scopes)
			if got := requestStatus(t, s.serviceB.Client(), tc.method, s.serviceB.URL+tc.path, token); got != 403 {
				t.Fatalf("status = %d, want 403", got)
			}
		})
	}
}

func TestAuthenticationHeaderFailures(t *testing.T) {
	s := newStack(t, []string{"orders:reader"}, []string{"orders:read"}, "service-a")
	token := s.token(t, "service-a", []string{"orders:read"})
	for _, headers := range [][]string{nil, {"Bearer"}, {"Basic abc"}, {"Bearer " + token, "Bearer " + token}, {"Bearer  " + token}, {"Bearer malformed"}, {"Bearer " + strings.Repeat("a", auth.MaxTokenBytes+1)}, {"Bearer " + token + ",Bearer " + token}} {
		req, err := http.NewRequestWithContext(t.Context(), "GET", s.serviceB.URL+"/orders", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, header := range headers {
			req.Header.Add("Authorization", header)
		}
		response, err := s.serviceB.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 || response.Header.Get("WWW-Authenticate") == "" {
			t.Fatalf("invalid authentication returned %d", response.StatusCode)
		}
	}
}

func TestSigningKeyRotation(t *testing.T) {
	s := newStack(t, []string{"orders:reader"}, []string{"orders:read"}, "service-a")
	oldToken := s.token(t, "service-a", []string{"orders:read"})
	s.idp.publish(t, "key-b")
	s.clock.advance(1)
	if err := s.verifier.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.idp.activate("key-b")
	newToken := s.token(t, "service-a", []string{"orders:read"})
	for _, token := range []string{oldToken, newToken} {
		if _, err := s.verifier.Verify(t.Context(), token); err != nil {
			t.Fatal("overlapping signing key was rejected")
		}
	}
	s.clock.advance(13)
	s.idp.retire("key-a")
	if err := s.verifier.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.verifier.Verify(t.Context(), oldToken); err == nil {
		t.Fatal("expired retired token accepted")
	}
	if _, err := s.verifier.Verify(t.Context(), s.token(t, "service-a", []string{"orders:read"})); err != nil {
		t.Fatal("active signing key rejected after retirement")
	}
}

func TestNewSigningKeyTriggersBoundedRefresh(t *testing.T) {
	s := newStack(t, []string{"orders:reader"}, []string{"orders:read"}, "service-a")
	s.idp.publish(t, "key-b")
	s.clock.advance(1)
	s.idp.activate("key-b")
	if _, err := s.verifier.Verify(t.Context(), s.token(t, "service-a", []string{"orders:read"})); err != nil {
		t.Fatal("new kid was not refreshed")
	}
	if s.jwksCalls.Load() != 2 {
		t.Fatal("expected exactly one unknown-kid refresh")
	}
}

func TestStrictSignedJWTMatrixAndJWKSOutage(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clock := newClock()
	var requests atomic.Int64
	var unavailable atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(publicKeySet(publicJWK("fixture-key", &key.PublicKey)))
	}))
	defer server.Close()
	verifier, err := auth.NewVerifier(t.Context(), auth.VerifierConfig{Issuer: server.URL, Audience: "service-b", JWKSURL: server.URL, HTTPClient: server.Client(), Now: clock.now, MaxLifetime: 5 * time.Minute, MaxStaleness: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	base := func() jwt.MapClaims {
		return jwt.MapClaims{"iss": server.URL, "sub": "service-a", "client_id": "service-a", "aud": "service-b", "scope": "orders:read", "roles": []string{"orders:reader"}, "iat": clock.now().Unix(), "nbf": clock.now().Unix(), "exp": clock.now().Add(time.Minute).Unix(), "jti": "fixture-id"}
	}
	sign := func(claims jwt.MapClaims, header map[string]any) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["typ"], token.Header["kid"] = "at+jwt", "fixture-key"
		for key, value := range header {
			token.Header[key] = value
		}
		raw, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	valid := sign(base(), nil)
	if _, err := verifier.Verify(t.Context(), valid); err != nil {
		t.Fatal("valid signed token failed")
	}
	for _, tc := range []struct {
		name   string
		mutate func(jwt.MapClaims)
		header map[string]any
	}{
		{"expired", func(c jwt.MapClaims) {
			c["iat"] = clock.now().Add(-time.Minute).Unix()
			c["nbf"] = c["iat"]
			c["exp"] = clock.now().Add(-time.Second).Unix()
		}, nil},
		{"future nbf", func(c jwt.MapClaims) { c["nbf"] = clock.now().Add(time.Second).Unix() }, nil},
		{"future iat", func(c jwt.MapClaims) { c["iat"] = clock.now().Add(time.Second).Unix() }, nil},
		{"issuer", func(c jwt.MapClaims) { c["iss"] = "https://attacker.invalid" }, nil},
		{"audience", func(c jwt.MapClaims) { c["aud"] = "another-service" }, nil},
		{"missing exp", func(c jwt.MapClaims) { delete(c, "exp") }, nil},
		{"missing iat", func(c jwt.MapClaims) { delete(c, "iat") }, nil},
		{"missing subject", func(c jwt.MapClaims) { delete(c, "sub") }, nil},
		{"missing client ID", func(c jwt.MapClaims) { delete(c, "client_id") }, nil},
		{"excessive lifetime", func(c jwt.MapClaims) { c["exp"] = clock.now().Add(6 * time.Minute).Unix() }, nil},
		{"wrong roles type", func(c jwt.MapClaims) { c["roles"] = "orders:reader" }, nil},
		{"ID token type", func(jwt.MapClaims) {}, map[string]any{"typ": "JWT"}},
		{"unknown kid", func(jwt.MapClaims) {}, map[string]any{"kid": "unknown"}},
		{"embedded jwk", func(jwt.MapClaims) {}, map[string]any{"jwk": publicJWK("attacker", &key.PublicKey)}},
		{"remote jku", func(jwt.MapClaims) {}, map[string]any{"jku": "https://attacker.invalid/jwks"}},
		{"remote x5u", func(jwt.MapClaims) {}, map[string]any{"x5u": "https://attacker.invalid/cert"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := base()
			tc.mutate(claims)
			if _, err := verifier.Verify(t.Context(), sign(claims, tc.header)); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	for _, raw := range []string{"malformed", "e30.e30.bad", valid[:len(valid)-8] + "AAAAAAAA"} {
		if _, err := verifier.Verify(t.Context(), raw); err == nil {
			t.Fatal("malformed signature accepted")
		}
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodNone, jwt.SigningMethodHS256} {
		token := jwt.NewWithClaims(method, base())
		token.Header["typ"], token.Header["kid"] = "at+jwt", "fixture-key"
		var signingKey any = []byte("test-only-hmac-not-trusted")
		if method == jwt.SigningMethodNone {
			signingKey = jwt.UnsafeAllowNoneSignatureType
		}
		raw, err := token.SignedString(signingKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifier.Verify(t.Context(), raw); err == nil {
			t.Fatal("unsupported algorithm accepted")
		}
	}
	clock.advance(1)
	before := requests.Load()
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := verifier.Verify(context.Background(), sign(base(), map[string]any{"kid": "random-unknown"})); err == nil {
				t.Error("unknown kid accepted")
			}
		}()
	}
	wg.Wait()
	if requests.Load()-before > 1 {
		t.Fatal("unknown-kid refresh storm")
	}
	unavailable.Store(true)
	clock.advance(1)
	if verifier.Refresh(t.Context()) == nil {
		t.Fatal("outage refresh succeeded")
	}
	if _, err := verifier.Verify(t.Context(), valid); err != nil {
		t.Fatal("last-known-good key not usable within staleness limit")
	}
	clock.advance(20)
	if verifier.Ready() {
		t.Fatal("stale keys reported ready")
	}
	if _, err := verifier.Verify(t.Context(), valid); err == nil {
		t.Fatal("stale key accepted during outage")
	}
	unavailable.Store(false)
	if err := verifier.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !verifier.Ready() {
		t.Fatal("cache failed to recover")
	}
}

func TestJWKSInvalidDocuments(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	valid := publicJWK("strong", &strong.PublicKey)
	for _, tc := range []struct {
		name string
		data any
	}{
		{"malformed", "not-a-jwks"}, {"empty", publicKeySet()},
		{"weak", publicKeySet(publicJWK("weak", &weak.PublicKey))},
		{"duplicate kid", publicKeySet(valid, valid)},
		{"oversized", strings.Repeat("x", 65537)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if text, ok := tc.data.(string); ok {
					io.WriteString(w, text)
					return
				}
				json.NewEncoder(w).Encode(tc.data)
			}))
			defer server.Close()
			verifier, err := auth.NewVerifier(t.Context(), auth.VerifierConfig{Issuer: server.URL, Audience: "service-b", JWKSURL: server.URL, HTTPClient: server.Client()})
			if err == nil {
				verifier.Close()
				t.Fatal("invalid JWKS accepted at startup")
			}
		})
	}
}
