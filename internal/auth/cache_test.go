package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticJWKSRefreshAndShutdown(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(publicKeySet{Keys: []publicJWK{encodePublicJWK("auto-refresh", &key.PublicKey)}})
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": {"max-age=2"}}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
		})}
		verifier, err := NewVerifier(t.Context(), VerifierConfig{Issuer: "https://fixture.invalid", JWKSURL: "https://fixture.invalid/keys", Audience: "service-b", HTTPClient: client})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatalf("scheduled calls = %d, want 2", calls.Load())
		}
		verifier.Close()
		before := calls.Load()
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if calls.Load() != before || verifier.Ready() {
			t.Fatal("closed verifier still active")
		}
	})
}

func TestOAuthConcurrentRenewalCloneAndOrigin(t *testing.T) {
	var tokenCalls, resourceCalls atomic.Int64
	var seconds atomic.Int64
	seconds.Store(1792000000)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenCalls.Add(1)
			if r.ParseForm() != nil || r.PostForm.Get("resource") != "service-b" {
				t.Error("resource request parameter lost")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "cache-fixture", "token_type": "Bearer", "expires_in": 10, "scope": "orders:read"})
			return
		}
		resourceCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer cache-fixture" {
			t.Error("token was not attached")
		}
		w.WriteHeader(503)
	}))
	defer server.Close()
	client, err := NewOAuthClient(OAuthClientConfig{TokenURL: server.URL + "/token", ResourceURL: server.URL, Resource: "service-b", ClientID: "service-a", ClientSecret: "test-only-client-secret", Scopes: []string{"orders:read"}, HTTPClient: server.Client(), Now: func() time.Time { return time.Unix(seconds.Load(), 0) }, RefreshBefore: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if token, err := client.AccessToken(t.Context()); err != nil || token != "cache-fixture" {
				t.Error("shared token source failed")
			}
		}()
	}
	close(start)
	wg.Wait()
	if tokenCalls.Load() != 1 {
		t.Fatal("concurrent token retrieval was not shared")
	}
	seconds.Add(8)
	if _, err := client.AccessToken(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatal("early renewal did not occur")
	}
	req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/orders", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "original")
	response, err := client.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if req.Header.Get("Authorization") != "original" || resourceCalls.Load() != 1 {
		t.Fatal("request mutation or unsafe API retry")
	}
	bad, err := http.NewRequestWithContext(t.Context(), "GET", "https://attacker.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RoundTrip(bad); err == nil {
		t.Fatal("access token forwarded to an untrusted host")
	}
	virtualHost, err := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/orders", nil)
	if err != nil {
		t.Fatal(err)
	}
	virtualHost.Host = "attacker.invalid"
	if _, err := client.RoundTrip(virtualHost); err == nil {
		t.Fatal("access token forwarded to an untrusted virtual host")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.AccessToken(canceled); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestOAuthWaitingCallerCanCancel(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, `{"access_token":"test-cache-token","token_type":"Bearer","expires_in":60,"scope":"read"}`)
	}))
	defer server.Close()
	client, err := NewOAuthClient(OAuthClientConfig{TokenURL: server.URL, ResourceURL: server.URL, Resource: "service-b", ClientID: "service-a", ClientSecret: "test-only-client-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done := make(chan error, 1)
	go func() { _, err := client.AccessToken(t.Context()); done <- err }()
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.AccessToken(ctx); err == nil {
		t.Error("canceled waiter accepted")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProductionEndpointConfiguration(t *testing.T) {
	for _, endpoint := range []string{"", "http://idp.invalid", "https://user:pass@idp.invalid", "https://idp.invalid/?redirect=attacker", "file:///keys", "https://idp.invalid/#fragment"} {
		if ValidateEndpoint(endpoint, false) == nil {
			t.Fatal("unsafe production endpoint accepted")
		}
	}
	if ValidateEndpoint("http://idp:8080/keys", true) != nil {
		t.Fatal("explicit isolated HTTP mode rejected")
	}
	if _, err := NewVerifier(t.Context(), VerifierConfig{Issuer: "https://idp.invalid", Audience: "b", JWKSURL: "https://idp.invalid/keys", Leeway: time.Minute}); err == nil {
		t.Fatal("unbounded leeway accepted")
	}
}
