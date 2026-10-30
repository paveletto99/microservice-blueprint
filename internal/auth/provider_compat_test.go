package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestExternalProviderMetadataAndClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := encodePublicJWK("external", &key.PublicKey)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kid": jwk.KeyID, "kty": "RSA", "use": "sig", "alg": "RS256", "n": jwk.Modulus, "e": jwk.Exponent,
			"x5c": []string{"unused-certificate-metadata"}, "x5t": "unused-thumbprint", "key_ops": []string{"verify"},
		}}, "provider_metadata": "ignored"})
	}))
	defer server.Close()
	verifier, err := NewVerifier(t.Context(), VerifierConfig{Issuer: server.URL, Audience: "service-b", JWKSURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal("standard provider JWKS metadata must be accepted", err)
	}
	defer verifier.Close()
	now := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": server.URL, "aud": "service-b", "sub": "provider-subject", "client_id": "service-a",
		"iat": now, "exp": now + 60, "scope": "orders:read", "roles": []string{"orders:reader"}, "provider_extra": "ignored",
	})
	token.Header["kid"], token.Header["typ"], token.Header["x5t"] = "external", "at+jwt", "ignored"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(t.Context(), raw)
	if err != nil || claims.ClientID != "service-a" || claims.Subject != "provider-subject" {
		t.Fatal("external subject and additional claims must not be rejected")
	}
}

func TestExplicitExternalClaimMapping(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(publicKeySet{Keys: []publicJWK{encodePublicJWK("mapped", &key.PublicKey)}})
	}))
	defer server.Close()
	mapping := ClaimsMapping{ClientID: "azp", Scopes: "scp", Roles: "/resource_access/service-b/roles", TokenUse: "token_use", TokenUseValue: "access"}
	verifier, err := NewVerifier(t.Context(), VerifierConfig{Issuer: server.URL, Audience: "service-b", JWKSURL: server.URL, HTTPClient: server.Client(), TokenType: "JWT", Claims: mapping, MaxLifetime: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	for _, tc := range []struct {
		name   string
		mutate func(jwt.MapClaims)
		valid  bool
	}{
		{"mapped", func(jwt.MapClaims) {}, true},
		{"no guessed fallback", func(c jwt.MapClaims) { delete(c, "azp"); c["client_id"] = "service-a" }, false},
		{"ID token marker", func(c jwt.MapClaims) { c["token_use"] = "id" }, false},
		{"missing marker", func(c jwt.MapClaims) { delete(c, "token_use") }, false},
		{"wrong mapped role type", func(c jwt.MapClaims) {
			c["resource_access"] = map[string]any{"service-b": map[string]any{"roles": "orders:reader"}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().Unix()
			claims := jwt.MapClaims{"iss": server.URL, "aud": "service-b", "sub": "provider-subject", "iat": now, "exp": now + 3600, "azp": "service-a", "scp": []string{"orders:read"}, "token_use": "access", "resource_access": map[string]any{"service-b": map[string]any{"roles": []string{"orders:reader"}}}}
			tc.mutate(claims)
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["typ"], token.Header["kid"] = "JWT", "mapped"
			raw, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			normalized, err := verifier.Verify(t.Context(), raw)
			if (err == nil) != tc.valid {
				t.Fatalf("verification success = %v, want %v", err == nil, tc.valid)
			}
			if tc.valid && !(Policy{Scopes: All("orders:read"), Roles: All("orders:reader"), Clients: Any("service-a")}).Allows(normalized) {
				t.Fatal("mapped claims did not satisfy policy")
			}
		})
	}
	if _, err := (ClaimsMapping{Roles: "/invalid~2pointer"}).configured(); err == nil {
		t.Fatal("invalid claim selector accepted")
	}
	if _, err := (ClaimsMapping{TokenUse: "token_use"}).configured(); err == nil {
		t.Fatal("partial token-use constraint accepted")
	}
}

func TestOAuthAuthenticationMethodsAndAudienceParameter(t *testing.T) {
	for _, method := range []string{ClientSecretBasic, ClientSecretPost} {
		t.Run(method, func(t *testing.T) {
			const secret = "test-only-special:%&secret"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ParseForm() != nil {
					t.Error("invalid token request")
				}
				if r.PostForm.Get("audience") != "service-b" || r.PostForm.Get("resource") != "" || r.PostForm.Get("scope") != "orders:read" {
					t.Error("wrong token parameters")
				}
				if method == ClientSecretBasic {
					id, encoded, ok := r.BasicAuth()
					id, idErr := url.QueryUnescape(id)
					decoded, secretErr := url.QueryUnescape(encoded)
					if !ok || idErr != nil || secretErr != nil || id != "service-a" || decoded != secret || r.PostForm.Get("client_secret") != "" {
						t.Error("incorrect Basic authentication")
					}
				} else if r.Header.Get("Authorization") != "" || r.PostForm.Get("client_id") != "service-a" || r.PostForm.Get("client_secret") != secret {
					t.Error("incorrect POST authentication")
				}
				json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-token", "token_type": "Bearer", "expires_in": 3600})
			}))
			defer server.Close()
			client, err := NewOAuthClient(OAuthClientConfig{TokenURL: server.URL, ResourceURL: server.URL, ClientID: "service-a", ClientSecret: secret, AuthMethod: method, Resource: "service-b", ResourceParameter: "audience", Scopes: []string{"orders:read"}, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err := client.AccessToken(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOAuthEndpointFailureDoesNotExposeResponse(t *testing.T) {
	for _, status := range []int{400, 401, 503} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":"fixture-private-response"}`))
		}))
		client, err := NewOAuthClient(OAuthClientConfig{TokenURL: server.URL, ResourceURL: server.URL, ClientID: "service-a", ClientSecret: "fixture-client-secret", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.AccessToken(t.Context()); err == nil || strings.Contains(err.Error(), "fixture-private-response") {
			t.Fatal("endpoint failure was accepted or disclosed response data")
		}
		client.Close()
		server.Close()
	}
}

func TestStandardOAuthRequestWithoutResource(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ParseForm() != nil {
			t.Error("invalid form")
		}
		if _, exists := r.PostForm["resource"]; exists {
			t.Error("unsolicited resource parameter")
		}
		if r.PostForm.Get("grant_type") != "client_credentials" {
			t.Error("missing grant")
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-token", "token_type": "Bearer", "expires_in": 3600, "provider_extension": true})
	}))
	defer server.Close()
	client, err := NewOAuthClient(OAuthClientConfig{TokenURL: server.URL, ResourceURL: server.URL, ClientID: "service-a", ClientSecret: "test-only-client-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal("resource must not be required for standard OAuth", err)
	}
	defer client.Close()
	if _, err := client.AccessToken(t.Context()); err != nil {
		t.Fatal(err)
	}
}
