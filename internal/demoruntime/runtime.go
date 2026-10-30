// Package demoruntime owns the demo processes' bounded HTTP lifecycle and health probes.
package demoruntime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// Env reads a configuration value without ever printing secrets or environment contents.
func Env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// Duration reads bounded typed duration settings; validation belongs to each module constructor.
func Duration(name string, fallback time.Duration) (time.Duration, error) {
	if os.Getenv(name) == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil {
		return 0, errors.New("demo configuration: invalid duration")
	}
	return value, nil
}

// Main attaches the shared logger and owns signal cancellation; startup stays in realMain.
func Main(realMain func(context.Context) error) {
	if len(os.Args) == 3 && os.Args[1] == "--healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Get(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		response.Body.Close() // Healthcheck response bodies are not needed.
		if response.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	logger := logging.DefaultLogger()
	defer logger.Sync()
	if err := realMain(logging.WithLogger(ctx, logger)); err != nil {
		logger.Errorw("demo process failed", "error", err)
		os.Exit(1)
	}
}

// HTTPServer describes a listener owned entirely by Serve.
type HTTPServer struct {
	Address string
	Handler http.Handler
}

// Serve owns one listener and joins its serving goroutine on shutdown.
func Serve(ctx context.Context, config HTTPServer) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return errors.New("demo server: listener startup failed")
	}
	defer listener.Close() // Idempotent close of the owned listener.
	server := &http.Server{Handler: middleware.Recovery()(config.Handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 * 1024, BaseContext: func(net.Listener) context.Context { return runCtx }}
	errorsCh := make(chan error, 1) // One terminal result; serving never blocks on shutdown.
	go func() { errorsCh <- server.Serve(listener) }()
	var result error
	stopped := false
	select {
	case <-ctx.Done():
	case err := <-errorsCh:
		stopped = true
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.New("demo server: serving failed")
		}
	}
	cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := server.Shutdown(shutdownCtx); err != nil {
		server.Close()
		result = errors.New("demo server: graceful shutdown timed out")
	}
	if !stopped {
		<-errorsCh
	}
	return result
}
