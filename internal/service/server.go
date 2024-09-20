/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"google.golang.org/grpc"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	payment "github.com/paveletto99/microservice-blueprint/pkg/api/payment/v1"
)

// Server is the admin server.
type Server struct {
	config *Config
	env    *serverenv.ServerEnv
}

// NewServer makes a new admin console server.
func NewServer(config *Config, env *serverenv.ServerEnv) (*Server, error) {
	// if env.Database() == nil {
	// 	return nil, fmt.Errorf("missing Database in server env")
	// }
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
	// mux.Handle("/", http.NotFoundHandler())
	mux.Handle("/healthz", HandleHealthz())
	return mux
}

func (s *Server) Run(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux)
	someMiddleware(mux)
	return mux
}

// grpc
func (s *Server) RunRpc(ctx context.Context) *grpc.Server {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", 8080))
	if err != nil {
		fmt.Errorf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	payment.RegisterPaymentServer(grpcServer, payment.UnimplementedPaymentServer{})
	grpcServer.Serve(listener)

	return grpcServer
}
