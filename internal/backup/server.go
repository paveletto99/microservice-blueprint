package backup

import (
	"context"
	"fmt"
	"net/http"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
)

type Server struct {
	config *Config
	env    *serverenv.ServerEnv
	db     *database.DB
	// h      *render.Renderer

	// overrideAuthToken is for testing to bypass API calls to get authentication
	// information.
	// overrideAuthToken string
}

func NewServer(config *Config, env *serverenv.ServerEnv) (*Server, error) {
	if env.Database() == nil {
		return nil, fmt.Errorf("missing database in server environment")
	}

	db := env.Database()

	return &Server{
		config: config,
		env:    env,
		db:     db,
	}, nil
}

func someMiddleware(handler http.Handler) http.Handler {
	// Example middleware that logs the request method and URL
	m := middleware.Recovery()
	handler = m(handler)
	return handler
}

func (s *Server) addRoutes(mux *http.ServeMux) *http.ServeMux {
	// mux.Handle("/", HandleBackup())
	mux.Handle("/healthz", server.HandleHealthz(s.db))
	return mux
}

func (s *Server) Run(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	s.addRoutes(mux)
	someMiddleware(mux)
	return mux
}
