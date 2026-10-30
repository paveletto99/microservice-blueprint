package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
)

func main() { demoruntime.Main(realMain) }

func realMain(ctx context.Context) error {
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
	handler, err := api.NewServiceBHandler(verifier, store, allowedClients...)
	if err != nil {
		return err
	}
	return demoruntime.Serve(ctx, demoruntime.HTTPServer{Address: demoruntime.Env("LISTEN_ADDRESS", ":8080"), Handler: handler})
}
