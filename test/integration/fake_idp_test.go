package integration_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
)

// This fixture is compiled only by go test. It has no discovery, admin or runtime command.
type fakeIDP struct {
	issuer, clientID string
	roles, scopes    []string
	clock            *clock
	mu               sync.RWMutex
	keys             map[string]*rsa.PrivateKey
	active           string
}

func newFakeIDP(t *testing.T, clock *clock, clientID string, roles, scopes []string) *fakeIDP {
	t.Helper()
	f := &fakeIDP{clientID: clientID, roles: append([]string(nil), roles...), scopes: append([]string(nil), scopes...), clock: clock, keys: make(map[string]*rsa.PrivateKey), active: "key-a"}
	f.publish(t, "key-a")
	return f
}

func (f *fakeIDP) publish(t *testing.T, kid string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.keys[kid] = key
	f.mu.Unlock()
}

func (f *fakeIDP) activate(kid string) { f.mu.Lock(); defer f.mu.Unlock(); f.active = kid }
func (f *fakeIDP) retire(kid string)   { f.mu.Lock(); defer f.mu.Unlock(); delete(f.keys, kid) }

func publicJWK(kid string, key *rsa.PublicKey) map[string]any {
	return map[string]any{"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()), "x5t": "ignored-provider-metadata"}
}

func publicKeySet(keys ...map[string]any) map[string]any { return map[string]any{"keys": keys} }

func (f *fakeIDP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" && r.URL.Path == "/.well-known/jwks.json" {
		f.mu.RLock()
		defer f.mu.RUnlock()
		ids := make([]string, 0, len(f.keys))
		for id := range f.keys {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		keys := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			keys = append(keys, publicJWK(id, &f.keys[id].PublicKey))
		}
		if err := json.NewEncoder(w).Encode(publicKeySet(keys...)); err != nil {
			http.Error(w, "fixture failed", 500)
		}
		return
	}
	if r.Method != "POST" || r.URL.Path != "/oauth/token" {
		http.NotFound(w, r)
		return
	}
	id, secret, ok := r.BasicAuth()
	id, idErr := url.QueryUnescape(id)
	secret, secretErr := url.QueryUnescape(secret)
	if !ok || idErr != nil || secretErr != nil || id != f.clientID || secret != fixtureSecret {
		http.Error(w, "invalid_client", 401)
		return
	}
	if r.ParseForm() != nil || r.PostForm.Get("grant_type") != "client_credentials" {
		http.Error(w, "invalid_request", 400)
		return
	}
	if target := r.PostForm.Get("resource"); target != "" && target != "service-b" {
		http.Error(w, "invalid_target", 400)
		return
	}
	scopes := strings.Fields(r.PostForm.Get("scope"))
	if len(scopes) == 0 {
		scopes = f.scopes
	}
	if !auth.All(scopes...).Match(f.scopes) {
		http.Error(w, "invalid_scope", 400)
		return
	}
	f.mu.RLock()
	kid, key, issuer := f.active, f.keys[f.active], f.issuer
	f.mu.RUnlock()
	now := f.clock.now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": issuer, "aud": "service-b", "sub": "external-subject", "client_id": f.clientID, "roles": f.roles, "scope": strings.Join(scopes, " "), "iat": now, "nbf": now, "exp": now + 10, "provider_extension": true})
	token.Header["typ"], token.Header["kid"] = "at+jwt", kid
	raw, err := token.SignedString(key)
	if err != nil {
		http.Error(w, "fixture signing failed", 500)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"access_token": raw, "token_type": "Bearer", "expires_in": 10, "scope": strings.Join(scopes, " "), "provider_extension": true}); err != nil {
		http.Error(w, "fixture failed", 500)
	}
}
