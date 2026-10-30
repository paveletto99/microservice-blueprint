// This package is the service that deletes old exposure keys; it is intended to be invoked over HTTP by Cloud Scheduler.
package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/paveletto99/microservice-blueprint/internal/backup"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
)

func main() {

	logger := logging.DefaultLogger()
	defer logger.Sync()

	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	ctx = logging.WithLogger(ctx, logger)

	defer func() {
		done()
		if r := recover(); r != nil {
			logger.Errorw("application panic", "panic", r)
		}
	}()
	err := realMain(ctx)
	done()

	if err != nil {
		logger.Errorw("application error", "error", err)
	}
	logger.Info("successful shutdown")
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
	logging.FromContext(ctx).Infow("listening", "port", config.Port)

	return srv.ServeHTTPHandler(ctx, backupServer.Run(ctx))
}
