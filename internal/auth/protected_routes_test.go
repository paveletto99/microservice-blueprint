package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type orderedTokenVerifier struct {
	calls  int
	claims *Claims
}

func (v *orderedTokenVerifier) Verify(context.Context, string) (*Claims, error) {
	v.calls++
	return v.claims, nil
}

func TestProtectedRoutesInstallAuthenticationBeforeAuthorization(t *testing.T) {
	verifier := &orderedTokenVerifier{claims: &Claims{ClientID: "service-a", Scope: "orders:read"}}
	routes, err := NewProtectedRoutes(verifier)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler, err := routes.Handler(Policy{Scopes: All("orders:read"), Clients: Any("service-a")}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		header string
		want   int
	}{{"", 401}, {"Bearer test", 204}} {
		request := httptest.NewRequest("GET", "/orders", nil)
		if tc.header != "" {
			request.Header.Set("Authorization", tc.header)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("status = %d, want %d", response.Code, tc.want)
		}
	}
	if calls != 1 || verifier.calls != 1 {
		t.Fatal("authentication and authorization order was not enforced")
	}
	if _, err := routes.Handler(Policy{}, http.NotFoundHandler()); err == nil {
		t.Fatal("empty policy was accepted")
	}
}
