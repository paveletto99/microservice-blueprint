package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.opencensus.io/plugin/ocgrpc"

	payment "github.com/paveletto99/microservice-blueprint/internal/payment"
	p "github.com/paveletto99/microservice-blueprint/internal/pb/payment"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
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
			slog.Log(
				context.Background(), slog.LevelError,
				fmt.Sprintf("😱 application panic: %s", r),
			)
		}
	}()
	err := realMain(ctx)
	done()

	if err != nil {
		logger.Log(
			context.Background(), logging.LevelFatal,
			fmt.Sprintf("😱 %s", err.Error()),
		)
	}
	logger.Info("successful shutdown")
}

func realMain(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	var config payment.Config

	env, err := setup.Setup(ctx, &config)
	if err != nil {
		return fmt.Errorf("setup.Setup: %w", err)
	}
	defer env.Close(ctx)

	payserver := payment.NewServer(env, &config)

	var sopts []grpc.ServerOption

	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("failed to create credentials: %w", err)
		}
		sopts = append(sopts, grpc.Creds(creds))
	}

	// if !config.AllowAnyClient {
	// sopts = append(sopts, grpc.UnaryInterceptor(federationServer.(*federationout.Server).AuthInterceptor))
	// }

	sopts = append(sopts, grpc.StatsHandler(&ocgrpc.ServerHandler{}))
	grpcServer := grpc.NewServer(sopts...)
	p.RegisterPaymentServer(grpcServer, payserver)

	srv, err := server.New(config.Port)
	if err != nil {
		return fmt.Errorf("server.New: %w", err)
	}
	logger.Info(fmt.Sprintf("listening on :%s", config.Port))

	return srv.ServeGRPC(ctx, grpcServer)
}
