package main

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
)

func main() { demoruntime.Main(realMain) }

func realMain(ctx context.Context) error {
	allowHTTP, err := strconv.ParseBool(demoruntime.Env("ALLOW_HTTP_RESOURCE", "false"))
	if err != nil {
		return err
	}
	resourceURL := os.Getenv("SERVICE_B_URL")
	oauth, err := auth.NewOAuthClient(auth.OAuthClientConfig{
		TokenURL: os.Getenv("OIDC_TOKEN_URL"), ResourceURL: resourceURL,
		Resource: os.Getenv("OAUTH_RESOURCE"), ResourceParameter: os.Getenv("OAUTH_RESOURCE_PARAMETER"),
		ClientID: os.Getenv("OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("OAUTH_CLIENT_SECRET"),
		AuthMethod: os.Getenv("OAUTH_AUTH_METHOD"), Scopes: strings.Fields(os.Getenv("OAUTH_SCOPES")), AllowHTTPResource: allowHTTP,
	})
	if err != nil {
		return err
	}
	defer oauth.Close()
	if _, err := oauth.AccessToken(ctx); err != nil {
		return err
	}
	return demoruntime.Serve(ctx, demoruntime.HTTPServer{Address: demoruntime.Env("LISTEN_ADDRESS", ":8080"), Handler: api.NewServiceAHandler(oauth, resourceURL)})
}
