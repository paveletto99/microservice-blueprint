package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/client"
	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
	"github.com/paveletto99/microservice-blueprint/internal/payment"
	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	transport "github.com/paveletto99/microservice-blueprint/pkg/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--grpc-healthcheck" {
		c, err := client.NewPaymentClient(os.Args[2])
		if err != nil {
			os.Exit(1)
		}
		defer c.Close()
		if c.Health(context.Background()) != nil {
			os.Exit(1)
		}
		return
	}
	demoruntime.Main(realMain)
}

type paymentHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	store *payment.PaymentRepository
}

func (h *paymentHealthServer) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if request.Service != "" && request.Service != "payment.v1.PaymentService" {
		return nil, status.Error(codes.NotFound, "unknown service")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	state := grpc_health_v1.HealthCheckResponse_SERVING
	if h.store.Health(ctx) != nil {
		state = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	return &grpc_health_v1.HealthCheckResponse{Status: state}, nil
}

func realMain(ctx context.Context) error {
	requests, err := setup.NewRequestRuntime(ctx, "payment")
	if err != nil {
		return err
	}
	defer func() {
		if err := requests.Close(context.Background()); err != nil {
			logging.FromContext(ctx).Error("request telemetry shutdown failed")
		}
	}()
	address := demoruntime.Env("PAYMENT_LISTEN_ADDRESS", "127.0.0.1:50051")
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("payment server: loopback listen address required")
	}
	cfg, err := database.VitessConfigFromEnv()
	if err != nil {
		return err
	}
	db, err := database.OpenVitess(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close(context.Background())
	poolMetrics, err := database.NewDatabasePoolMetrics(db.Pool)
	if err != nil {
		return err
	}
	if err := requests.Stack.RegisterMetrics(poolMetrics); err != nil {
		return err
	}
	store, err := payment.NewPaymentRepository(db.Pool)
	if err != nil {
		return err
	}
	if err := store.Health(ctx); err != nil {
		return errors.New("payment schema unavailable; run payment migration")
	}
	if err := requests.Stack.RegisterDependency("database", store.Health); err != nil {
		return err
	}
	metricsAddress := demoruntime.Env("PAYMENT_METRICS_ADDRESS", "127.0.0.1:9090")
	metricsHost, _, err := net.SplitHostPort(metricsAddress)
	if err != nil || net.ParseIP(metricsHost) == nil || !net.ParseIP(metricsHost).IsLoopback() {
		return errors.New("payment server: loopback metrics address required")
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(4096), grpc.MaxSendMsgSize(4096), grpc.ChainUnaryInterceptor(requests.Stack.UnaryServerInterceptor()))
	paymentv1.RegisterPaymentServiceServer(server, payment.NewPaymentServer(store))
	grpc_health_v1.RegisterHealthServer(server, &paymentHealthServer{store: store})
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", requests.Stack.OperationalHTTP("GET /metrics", requests.Stack.MetricsHandler()))
	return transport.ServeGRPCAndHTTPContext(ctx, address, server, transport.HTTPContextConfig{Address: metricsAddress, Handler: requests.Stack.HTTP(mux)})
}
