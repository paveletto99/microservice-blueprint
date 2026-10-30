package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizerFreezesPolicyAndDeniesWithoutHandler(t *testing.T) {
	policy := Policy{Scopes: All("read"), Roles: Any("reader"), Clients: Any("service-a")}
	called := 0
	handler, err := (Authorizer{}).Protect(policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(204) }))
	if err != nil {
		t.Fatal(err)
	}
	policy.Scopes.Values[0] = "changed"
	for _, tc := range []struct {
		claims *Claims
		status int
	}{{nil, 401}, {&Claims{ClientID: "service-a", Scope: "read", Roles: []string{"other"}}, 403}, {&Claims{ClientID: "service-a", Scope: "read", Roles: []string{"reader"}}, 204}} {
		ctx := context.Background()
		if tc.claims != nil {
			ctx = context.WithValue(ctx, claimsContextKey{}, tc.claims)
		}
		req := httptest.NewRequestWithContext(ctx, "GET", "/", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("status = %d, want %d", response.Code, tc.status)
		}
	}
	if called != 1 {
		t.Fatal("denied request invoked business handler")
	}
	if _, err := (Authorizer{}).Protect(Policy{}, http.NotFoundHandler()); err == nil {
		t.Fatal("empty route policy registered")
	}
}

func TestRequirementMatch(t *testing.T) {
	for _, tc := range []struct {
		requirement Requirement
		actual      []string
		want        bool
	}{{Any("a", "b"), []string{"b", "b"}, true}, {All("a", "b"), []string{"a"}, false}, {All("a", "b"), []string{"b", "a"}, true}, {Any(), nil, true}, {Any("a"), []string{"A"}, false}, {Any("a", "a"), []string{"a"}, false}} {
		if got := tc.requirement.Match(tc.actual); got != tc.want {
			t.Fatalf("requirement match = %v, want %v", got, tc.want)
		}
	}
}
