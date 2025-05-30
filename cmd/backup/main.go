// This package is the service that deletes old exposure keys; it is intended to be invoked over HTTP by Cloud Scheduler.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/paveletto99/microservice-blueprint/internal/backup"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
)

func main() {

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		done()
		if r := recover(); r != nil {
			slog.Error("😱 application panic", "panic", r)
		}
	}()
	err := realMain(ctx)
	done()

	if err != nil {
		slog.ErrorContext(ctx, "😱 application error", "error", err)
	}
	slog.Info("successful shutdown")
}

func realMain(ctx context.Context) error {

	var config backup.Config
	env, err := setup.Setup(ctx, &config)
	if err != nil {
		return fmt.Errorf("setup.Setup: %w", err)
	}
	defer env.Close(ctx)

	backupServer, err := backup.NewServer(&config, env)
	if err != nil {
		return fmt.Errorf("backup.NewServer: %w", err)
	}

	srv, err := server.New(config.Port)
	if err != nil {
		return fmt.Errorf("server.New: %w", err)
	}
	slog.Info("listening on: ", config.Port)

	return srv.ServeHTTPHandler(ctx, backupServer.Run(ctx))
}
