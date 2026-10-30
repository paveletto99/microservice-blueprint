package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// TokenVerifier verifies signatures and claims before any authorization decisions.
type TokenVerifier interface {
	Verify(context.Context, string) (*Claims, error)
}

// Authenticator authenticates exactly one bounded Bearer authorization header.
type Authenticator struct{ verifier TokenVerifier }

// NewAuthenticator requires a verifier; it never parses tokens independently of it.
func NewAuthenticator(verifier TokenVerifier) (*Authenticator, error) {
	if verifier == nil {
		return nil, errors.New("auth middleware: verifier is required")
	}
	return &Authenticator{verifier}, nil
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="service-b"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// Authenticate inserts verified claims into a private context key, or returns 401.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 || len(headers[0]) > MaxTokenBytes+7 {
			unauthorized(w)
			return
		}
		parts := strings.Split(headers[0], " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			unauthorized(w)
			return
		}
		claims, err := a.verifier.Verify(r.Context(), parts[1])
		if err != nil || claims == nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsContextKey{}, claims)))
	})
}
