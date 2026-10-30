package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/internal/service"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	logger := logging.DefaultLogger()
	defer logger.Sync()
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

	var config service.Config

	// env, err := setup.Setup(ctx, &config)
	// if err != nil {
	// 	return fmt.Errorf("setup.Setup: %w", err)
	// }
	// defer env.Close(ctx)
	env := &serverenv.ServerEnv{}
	serviceServer, err := service.NewServer(&config, env)
	if err != nil {
		return fmt.Errorf("service.NewServer: %w", err)
	}

	srv, err := server.New(config.Port)
	if err != nil {
		return fmt.Errorf("server.New: %w", err)
	}
	logging.FromContext(ctx).Infow("listening", "port", config.Port)

	return srv.ServeHTTPHandler(ctx, serviceServer.Run(ctx))
}
