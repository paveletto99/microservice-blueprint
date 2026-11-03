package auth

import "net/http"

// ProtectedRoutes installs authentication and frozen authorization together at registration.
// The HTTP adapter cannot accidentally authorize before verifying the Bearer token.
type ProtectedRoutes struct{ authenticator *Authenticator }

// NewProtectedRoutes reuses the existing verifier rather than introducing another token parser.
func NewProtectedRoutes(verifier TokenVerifier) (*ProtectedRoutes, error) {
	authenticator, err := NewAuthenticator(verifier)
	if err != nil {
		return nil, err
	}
	return &ProtectedRoutes{authenticator: authenticator}, nil
}

// Handler validates route policy once and guarantees authentication precedes authorization.
func (r *ProtectedRoutes) Handler(policy Policy, next http.Handler) (http.Handler, error) {
	handler, err := (Authorizer{}).Protect(policy, next)
	if err != nil {
		return nil, err
	}
	return r.authenticator.Authenticate(handler), nil
}
