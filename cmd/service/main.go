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
	logger := logging.NewLogger("Info", true)

	ctx = logging.WithLogger(ctx, logger)

	defer func() {
		done()
		if r := recover(); r != nil {
			logger.Error("😱 application panic", "panic", r)
		}
	}()
	err := realMain(ctx)
	done()

	if err != nil {
		logger.Log(ctx, logging.LevelFatal, err.Error())
	}
	logger.Info("successful shutdown")
}

func realMain(ctx context.Context) error {
	logger := logging.FromContext(ctx)

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
	logger.Info(fmt.Sprintf("listening on :%s", config.Port))

	return srv.ServeHTTPHandler(ctx, serviceServer.Run(ctx))
}
