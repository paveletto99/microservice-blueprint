// Package api composes the service-to-service demo's business routes and policies.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
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
// Deprecated: production startup uses BuildServiceBHandler with an explicit request stack.
func NewServiceBHandler(verifier *auth.Verifier, store orders.Store, allowedClients ...string) (http.Handler, error) {
	requests, err := defaultServiceRequests("service-b")
	if err != nil {
		return nil, err
	}
	return BuildServiceBHandler(ServiceBHandlerConfig{Verifier: verifier, Orders: store, AllowedClients: allowedClients, Requests: requests})
}

// ServiceBHandlerConfig makes request middleware and business dependencies explicit at startup.
type ServiceBHandlerConfig struct {
	Verifier       *auth.Verifier
	Orders         orders.Store
	Payments       PaymentCaller
	AllowedClients []string
	Requests       *middleware.RequestStack
}

// BuildServiceBHandler composes operational routes and protected business routes exactly once.
func BuildServiceBHandler(config ServiceBHandlerConfig) (http.Handler, error) {
	verifier, store, payments, allowedClients, requests := config.Verifier, config.Orders, config.Payments, config.AllowedClients, config.Requests
	if verifier == nil || store == nil {
		return nil, errors.New("API: verifier and order store are required")
	}
	if len(allowedClients) == 0 {
		return nil, errors.New("API: an explicit client allowlist is required")
	}
	if requests == nil {
		return nil, errors.New("API: request stack is required")
	}
	protected, err := auth.NewProtectedRoutes(verifier)
	if err != nil {
		return nil, err
	}
	if err := requests.RegisterMetrics(auth.VerifierMetrics(verifier)...); err != nil {
		return nil, err
	}
	if err := requests.RegisterDependency("jwks", func(context.Context) error {
		if !verifier.Ready() {
			return errors.New("verifier not ready")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := requests.RegisterDependency("database", store.Health); err != nil {
		return nil, err
	}
	if payments != nil {
		if err := requests.RegisterDependency("payment", payments.Health); err != nil {
			return nil, err
		}
	}
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", requests.OperationalHTTP("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })))
	mux.Handle("GET /readyz", requests.OperationalHTTP("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !verifier.Ready() || store.Health(ctx) != nil || (payments != nil && payments.Health(ctx) != nil) {
			http.Error(w, "not ready", 503)
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})))
	mux.Handle("GET /metrics", requests.OperationalHTTP("GET /metrics", requests.MetricsHandler()))
	handlers := orderHandlers{store: store}
	for _, route := range []struct {
		pattern string
		policy  auth.Policy
		handler http.HandlerFunc
	}{
		{"GET /orders", auth.Policy{Scopes: auth.All("orders:read"), Roles: auth.Any("orders:reader", "orders:admin"), Clients: auth.Any(allowedClients...)}, handlers.list},
		{"GET /orders/{id}", auth.Policy{Scopes: auth.All("orders:read"), Roles: auth.Any("orders:reader", "orders:admin"), Clients: auth.Any(allowedClients...)}, handlers.get},
		{"POST /orders", auth.Policy{Scopes: auth.All("orders:write"), Roles: auth.Any("orders:writer", "orders:admin"), Clients: auth.Any(allowedClients...)}, handlers.create},
		{"PUT /orders/{id}", auth.Policy{Scopes: auth.All("orders:write"), Roles: auth.Any("orders:writer", "orders:admin"), Clients: auth.Any(allowedClients...)}, handlers.update},
		{"DELETE /orders/{id}", auth.Policy{Scopes: auth.All("orders:delete"), Roles: auth.All("orders:admin", "orders:delete"), Clients: auth.Any(allowedClients...)}, handlers.delete},
	} {
		handler, err := protected.Handler(route.policy, route.handler)
		if err != nil {
			return nil, err
		}
		mux.Handle(route.pattern, requests.BusinessHTTP(route.pattern, handler))
	}
	if payments != nil {
		paymentHandler := paymentHandlers{store: store, payments: payments}
		policy := auth.Policy{Scopes: auth.All("payments:write"), Roles: auth.Any("payments:writer", "payments:admin"), Clients: auth.Any(allowedClients...)}
		handler, err := protected.Handler(policy, http.HandlerFunc(paymentHandler.create))
		if err != nil {
			return nil, err
		}
		mux.Handle("POST /orders/{id}/payments", requests.BusinessHTTP("POST /orders/{id}/payments", handler))
	}
	return requests.HTTP(mux), nil
}
