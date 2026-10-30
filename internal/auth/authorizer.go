package auth

import (
	"errors"
	"net/http"
)

// Authorizer owns only authorization; authentication must run before Protect handlers.
type Authorizer struct{}

// Protect validates and freezes policy at route registration, not at request time.
func (Authorizer) Protect(policy Policy, next http.Handler) (http.Handler, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if next == nil {
		return nil, errors.New("auth policy: handler is required")
	}
	policy = policy.frozen()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		if !policy.Allows(claims) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}
