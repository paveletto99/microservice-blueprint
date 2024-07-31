/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/paveletto99/go-pobo/pkg/api"
	"github.com/quic-go/quic-go/http3"
	"github.com/sirupsen/logrus"
)

// Compile check *Pobo implements Runner interface
var _ api.Runner = &Server{}

type Handler = http.Handler

type Server struct {
	log    *logrus.Logger
	config *Config
	// env *serverenv.ServerEnv
}

func NewServer(logger *logrus.Logger, config *Config) (*Server, error) {
	return &Server{log: logger, config: config}, nil
}

func someMiddleware(handler http.Handler) http.Handler {
	return handler
}

func addRoutes(mux *http.ServeMux, logger *logrus.Logger) *http.ServeMux {
	logger.Info("unimplemented")
	mux.Handle("/", http.NotFoundHandler())
	mux.Handle("/healthz", HandleHealthz(logger))
	return mux
}

func (s *Server) Run(ctx context.Context) error {
	client := api.Client{}
	server := api.Server{}

	mux := http.NewServeMux()
	addRoutes(mux, s.log)
	someMiddleware(mux)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	s.log.Infof("Client: %x", client)
	s.log.Infof("Server: %x", server)

	httpServer := &http3.Server{
		Addr:    net.JoinHostPort("127.0.0.1", s.config.Port),
		Handler: mux,
	}

	go func() {
		s.log.Printf("listening on %s\n", httpServer.Addr)
		if err := httpServer.ListenAndServeTLS("./tools/certs/certificate.pem", "./tools/certs/certificate.key"); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "error listening and serving: %s\n", err)
		}
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done() // make a new context for the Shutdown
		// shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		// defer cancel()
		if err := httpServer.CloseGracefully(10 * time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "error shutting down http server: %s\n", err)
		}
	}()

	wg.Wait()

	return nil
}
