package client

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PaymentClient owns a reusable gRPC connection to a loopback Envoy payment proxy.
type PaymentClient struct {
	conn *grpc.ClientConn
	rpc  paymentv1.PaymentServiceClient
}

// NewPaymentClient allows plaintext only to a loopback Envoy in the same workload namespace.
func NewPaymentClient(address string) (*PaymentClient, error) {
	return NewPaymentClientWithInterceptor(address, nil)
}

// NewPaymentClientWithInterceptor adds explicit client observation without changing TLS rules.
func NewPaymentClientWithInterceptor(address string, interceptor grpc.UnaryClientInterceptor) (*PaymentClient, error) {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	n, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 {
		return nil, errors.New("payment client: loopback proxy address required")
	}
	options := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if interceptor != nil {
		options = append(options, grpc.WithChainUnaryInterceptor(interceptor))
	}
	conn, err := grpc.NewClient(address, options...)
	if err != nil {
		return nil, errors.New("payment client: invalid proxy configuration")
	}
	return &PaymentClient{conn: conn, rpc: paymentv1.NewPaymentServiceClient(conn)}, nil
}

// CreatePayment sends one bounded payment RPC without automatic retries.
func (c *PaymentClient) CreatePayment(ctx context.Context, request *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	return c.rpc.CreatePayment(ctx, request)
}

// Health checks the payment service and its store with a bounded RPC.
func (c *PaymentClient) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	response, err := grpc_health_v1.NewHealthClient(c.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "payment.v1.PaymentService"})
	if err != nil || response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return errors.New("payment client: service not ready")
	}
	return nil
}

// Close releases the payment gRPC connection.
func (c *PaymentClient) Close() error { return c.conn.Close() }
