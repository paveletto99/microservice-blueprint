// Package demoruntime owns the demo processes' bounded HTTP lifecycle and health probes.
package demoruntime

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
