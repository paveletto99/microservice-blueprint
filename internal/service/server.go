/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/pprof"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/utils"
)

// Server is the admin server.
type Server struct {
	config *Config
	env    *serverenv.ServerEnv
}

// NewServer makes a new admin console server.
func NewServer(config *Config, env *serverenv.ServerEnv) (*Server, error) {
	if env.Database() == nil {
		return nil, fmt.Errorf("missing Database in server env")
	}
	if config.ProfilingEnabled {
		p, _ := os.Create("/tmp/pprof.prof")
		pprof.StartCPUProfile(p)
		defer p.Close()
		defer pprof.StopCPUProfile()
	}

	utils.Assert(config != nil, "config must not be nil")
	return &Server{
		config: config,
		env:    env,
	}, nil
}

type Handler = http.Handler

func someMiddleware(handler http.Handler) http.Handler {
	return handler
}

func addRoutes(mux *http.ServeMux) *http.ServeMux {
	mux.Handle("/", http.NotFoundHandler())
	mux.Handle("/healthz", HandleHealthz())
	return mux
}

func (s *Server) Run(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux)
	someMiddleware(mux)
	return mux
}
