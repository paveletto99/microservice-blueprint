package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/paveletto99/go-pobo/internal/serverenv"
	"github.com/paveletto99/go-pobo/internal/service"
	"github.com/paveletto99/go-pobo/pkg/logging"
	"github.com/paveletto99/go-pobo/pkg/server"
	// "github.com/paveletto99/go-pobo/utils"
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	logger := logging.NewLogger("Info", true)

	ctx = logging.WithLogger(ctx, logger)

	defer func() {
		done()
		if r := recover(); r != nil {
			logger.LogAttrs(
				context.Background(), logging.LevelPanic,
				"😱 application panic",
				// r,
			)
		}
	}()
	err := realMain(ctx)
	done()

	if err != nil {
		// logger.Fatal(err)
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
	logger.Info("listening on :%s", config.Port)

	return srv.ServeGRPC(ctx, serviceServer.RunRpc(ctx))
}
