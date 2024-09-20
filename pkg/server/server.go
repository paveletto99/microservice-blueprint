package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/paveletto99/go-pobo/pkg/logging"
	"github.com/paveletto99/go-pobo/pkg/observability"
	"github.com/quic-go/quic-go/http3"
	"google.golang.org/grpc"
)

// Server provides a gracefully-stoppable http server implementation. It is safe
// for concurrent use in goroutines.
type Server struct {
	ip       string
	port     string
	listener net.Listener
}

// New creates a new server listening on the provided address that responds to
// the http.Handler. It starts the listener, but does not start the server. If
// an empty port is given, the server randomly chooses one.
func New(port string) (*Server, error) {
	// Create the net listener first, so the connection ready when we return. This
	// guarantees that it can accept requests.
	addr := fmt.Sprintf(":" + port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener on %s: %w", addr, err)
	}

	return &Server{
		ip:       listener.Addr().(*net.TCPAddr).IP.String(),
		port:     strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
		listener: listener,
	}, nil
}

// NewFromListener creates a new server on the given listener. This is useful if
// you want to customize the listener type (e.g. udp or tcp) or bind network
// more than `New` allows.
func NewFromListener(listener net.Listener) (*Server, error) {
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("listener is not tcp")
	}

	return &Server{
		ip:       addr.IP.String(),
		port:     strconv.Itoa(addr.Port),
		listener: listener,
	}, nil
}

func (s *Server) ServeHTTP3(ctx context.Context, srv *http3.Server) error {
	logger := logging.FromContext(ctx)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	// Create the prometheus metrics proxy.
	metricsDone, err := observability.ServeMetricsIfPrometheus(ctx)
	if err != nil {
		return fmt.Errorf("failed to serve metrics: %w", err)
	}

	go func() {
		logger.Info("listening on %s\n", srv.Addr)
		if err := srv.ListenAndServeTLS("./tools/certs/certificate.pem", "./tools/certs/certificate.key"); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "error listening and serving: %s\n", err)
		}

	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done() // make a new context for the Shutdown
		if err := srv.CloseGracefully(10 * time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "error shutting down http server: %s\n", err)
		}
	}()

	wg.Wait()

	// Shutdown the prometheus metrics proxy.
	if metricsDone != nil {
		if err := metricsDone(); err != nil {
			logger.Error("failed to close metrics exporter: %w", err)
		}
	}

	return nil
}

// ServeHTTP starts the server and blocks until the provided context is closed.
// When the provided context is closed, the server is gracefully stopped with a
// timeout of 5 seconds.
//
// Once a server has been stopped, it is NOT safe for reuse.
func (s *Server) ServeHTTP(ctx context.Context, srv *http.Server) error {
	logger := logging.FromContext(ctx)

	// Spawn a goroutine that listens for context closure. When the context is
	// closed, the server is stopped.
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()

		logger.Debug("server.Serve: context closed")
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()

		logger.Debug("server.Serve: shutting down")
		errCh <- srv.Shutdown(shutdownCtx)
	}()

	// // Create the prometheus metrics proxy.
	// metricsDone, err := ServeMetricsIfPrometheus(ctx)
	// if err != nil {
	// 	return fmt.Errorf("failed to serve metrics: %w", err)
	// }

	// Run the server. This will block until the provided context is closed.
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to serve: %w", err)
	}

	// logger.Debugf("server.Serve: serving stopped")

	// var merr *multierror.Error

	// // Shutdown the prometheus metrics proxy.
	// if metricsDone != nil {
	// 	if err := metricsDone(); err != nil {
	// 		merr = multierror.Append(merr, fmt.Errorf("failed to close metrics exporter: %w", err))
	// 	}
	// }

	// Return any errors that happened during shutdown.
	// if err := <-errCh; err != nil {
	// 	merr = multierror.Append(merr, fmt.Errorf("failed to shutdown server: %w", err))
	// }
	// return merr.ErrorOrNil()
	return nil
}

// ServeHTTPHandler is a convenience wrapper around ServeHTTP. It creates an
// HTTP server using the provided handler, wrapped in OpenCensus for
// observability.
func (s *Server) ServeHTTPHandler(ctx context.Context, handler http.Handler) error {
	return s.ServeHTTP3(ctx, &http3.Server{
		Addr:    net.JoinHostPort(s.ip, s.port),
		Handler: handler,
	})
	// return s.ServeHTTP(ctx, &http.Server{
	// 	ReadHeaderTimeout: 10 * time.Second,
	// 	Handler: &ochttp.Handler{
	// 		Handler:          handler,
	// 		IsPublicEndpoint: true,
	// 		Propagation:      &tracecontext.HTTPFormat{},
	// 	},
	// })
}

// ServeGRPC starts the server and blocks until the provided context is closed.
// When the provided context is closed, the server is gracefully stopped with a
// timeout of 5 seconds.
//
// Once a server has been stopped, it is NOT safe for reuse.
func (s *Server) ServeGRPC(ctx context.Context, srv *grpc.Server) error {
	logger := logging.FromContext(ctx)

	// Spawn a goroutine that listens for context closure. When the context is
	// closed, the server is stopped.
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()

		logger.Debug("server.Serve: context closed")
		logger.Debug("server.Serve: shutting down")
		srv.GracefulStop()
	}()

	// Run the server. This will block until the provided context is closed.
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("failed to serve: %w", err)
	}

	logger.Debug("server.Serve: serving stopped")

	// Return any errors that happened during shutdown.
	select {
	case err := <-errCh:
		return fmt.Errorf("failed to shutdown: %w", err)
	default:
		return nil
	}
}

// Addr returns the server's listening address (ip + port).
func (s *Server) Addr() string {
	return net.JoinHostPort(s.ip, s.port)
}

// IP returns the server's listening IP.
func (s *Server) IP() string {
	return s.ip
}

// Port returns the server's listening port.
func (s *Server) Port() string {
	return s.port
}
