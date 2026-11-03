package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/client"
	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
)

func main() { demoruntime.Main(realMain) }

func realMain(ctx context.Context) error {
	requests, err := setup.NewRequestRuntime(ctx, "service-b")
	if err != nil {
		return err
	}
	defer func() {
		if err := requests.Close(context.Background()); err != nil {
			logging.FromContext(ctx).Error("request telemetry shutdown failed")
		}
	}()
	allowedClients := strings.Fields(os.Getenv("AUTH_ALLOWED_CLIENTS"))
	if len(allowedClients) == 0 {
		return errors.New("auth configuration: AUTH_ALLOWED_CLIENTS is required")
	}
	if err := (auth.Policy{Clients: auth.Any(allowedClients...)}).Validate(); err != nil {
		return err
	}
	dbConfig, err := database.VitessConfigFromEnv()
	if err != nil {
		return err
	}
	db, err := database.OpenVitess(ctx, dbConfig)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	poolMetrics, err := database.NewDatabasePoolMetrics(db.Pool)
	if err != nil {
		return err
	}
	if err := requests.Stack.RegisterMetrics(poolMetrics); err != nil {
		return err
	}
	store, err := orders.NewRepository(db.Pool)
	if err != nil {
		return err
	}
	if err := store.Health(ctx); err != nil {
		return errors.New("orders schema unavailable; run the migration job before starting Service B")
	}
	lifetime, err := demoruntime.Duration("JWT_MAX_LIFETIME", 10*time.Minute)
	if err != nil {
		return err
	}
	verifier, err := auth.NewVerifier(ctx, auth.VerifierConfig{
		Issuer: os.Getenv("OIDC_ISSUER"), Audience: os.Getenv("OAUTH_AUDIENCE"), JWKSURL: os.Getenv("OIDC_JWKS_URL"),
		TokenType: os.Getenv("JWT_TOKEN_TYPE"), MaxLifetime: lifetime, Leeway: 5 * time.Second,
		Claims: auth.ClaimsMapping{ClientID: os.Getenv("JWT_CLIENT_ID_CLAIM"), Scopes: os.Getenv("JWT_SCOPE_CLAIM"), Roles: os.Getenv("JWT_ROLES_CLAIM"), TokenUse: os.Getenv("JWT_TOKEN_USE_CLAIM"), TokenUseValue: os.Getenv("JWT_TOKEN_USE_VALUE")},
	})
	if err != nil {
		return err
	}
	defer verifier.Close()
	config := api.ServiceBHandlerConfig{Verifier: verifier, Orders: store, AllowedClients: allowedClients, Requests: requests.Stack}
	if address := os.Getenv("PAYMENT_GRPC_ADDRESS"); address != "" {
		payments, err := client.NewPaymentClientWithInterceptor(address, requests.Stack.UnaryClientInterceptor())
		if err != nil {
			return err
		}
		defer payments.Close()
		config.Payments = payments
	}
	handler, err := api.BuildServiceBHandler(config)
	if err != nil {
		return err
	}
	return server.ServeHTTPContext(ctx, server.HTTPContextConfig{Address: demoruntime.Env("LISTEN_ADDRESS", ":8080"), Handler: handler})
}
