// Package api composes the service-to-service demo's business routes and policies.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

func jsonResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logging.DefaultLogger().Warn("demo API response could not be encoded")
	}
}

// NewServiceBHandler keeps liveness public and puts authentication before frozen route policies.
func NewServiceBHandler(verifier *auth.Verifier, allowedClients ...string) (http.Handler, error) {
	if verifier == nil {
		return nil, errors.New("demo API: verifier is required")
	}
	if len(allowedClients) == 0 {
		return nil, errors.New("API: an explicit client allowlist is required")
	}
	authenticator, err := auth.NewAuthenticator(verifier)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !verifier.Ready() {
			http.Error(w, "not ready", 503)
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		age, count, ready := verifier.CacheStatus()
		readyNumber := 0
		if ready {
			readyNumber = 1
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		if _, err := fmt.Fprintf(w, "auth_jwks_cache_age_seconds %g\nauth_jwks_keys %d\nauth_jwks_ready %d\n", age.Seconds(), count, readyNumber); err != nil {
			logging.FromContext(r.Context()).Warn("cache metrics response failed")
		}
	})
	authorizer := auth.Authorizer{}
	for _, route := range []struct {
		pattern string
		policy  auth.Policy
		status  int
	}{
		{"GET /orders", auth.Policy{Scopes: auth.All("orders:read"), Roles: auth.Any("orders:reader", "orders:admin"), Clients: auth.Any(allowedClients...)}, 200},
		{"POST /orders", auth.Policy{Scopes: auth.All("orders:write"), Roles: auth.Any("orders:writer", "orders:admin"), Clients: auth.Any(allowedClients...)}, 201},
		{"DELETE /orders/{id}", auth.Policy{Scopes: auth.All("orders:delete"), Roles: auth.All("orders:admin", "orders:delete"), Clients: auth.Any(allowedClients...)}, 204},
	} {
		status := route.status
		handler, err := authorizer.Protect(route.policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
				if err != nil || len(body) > 4096 {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
			}
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", 401)
				return
			}
			if status == 204 {
				w.WriteHeader(status)
				return
			}
			jsonResponse(w, status, map[string]any{"caller": claims.ClientID, "orders": []map[string]string{{"id": "demo-1", "item": "example"}}})
		}))
		if err != nil {
			return nil, err
		}
		mux.Handle(route.pattern, authenticator.Authenticate(handler))
	}
	return mux, nil
}
