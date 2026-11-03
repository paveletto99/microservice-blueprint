package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
)

// HTTPContextConfig describes an explicitly composed handler; no middleware is installed here.
type HTTPContextConfig struct {
	Address string
	Handler http.Handler
}

// ServeHTTPContext owns a TCP HTTP listener and joins serving and bounded shutdown.
// This is the canonical A/B lifecycle. Applications serve plaintext on loopback;
// mesh mTLS and HTTP/3 terminate in the workload's Envoy (docs/spiffe-envoy.md).
func ServeHTTPContext(ctx context.Context, config HTTPContextConfig) error {
	if config.Handler == nil {
		return errors.New("HTTP server: handler is required")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return errors.New("HTTP server: listener startup failed")
	}
	defer listener.Close()
	server := &http.Server{Handler: config.Handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 * 1024, BaseContext: func(net.Listener) context.Context { return runCtx }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	var result error
	stopped := false
	select {
	case <-ctx.Done():
	case err := <-done:
		stopped = true
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.New("HTTP server: serving failed")
		}
	}
	cancel()
	shutdownCtx, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		result = errors.New("HTTP server: graceful shutdown timed out")
	}
	if !stopped {
		<-done
	}
	return result
}

// ServeGRPCContext owns a gRPC listener and bounds graceful draining to five seconds.
// Interceptors and transport credentials must be configured before calling it.
func ServeGRPCContext(ctx context.Context, address string, server *grpc.Server) error {
	if server == nil {
		return errors.New("gRPC server: server is required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("gRPC server: listener startup failed")
	}
	defer listener.Close()
	defer server.Stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return errors.New("gRPC server: serving failed")
		}
		return nil
	case <-ctx.Done():
		stopped := make(chan struct{})
		go func() { server.GracefulStop(); close(stopped) }()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-stopped:
		case <-timer.C:
			server.Stop()
			<-stopped
		}
		<-done
		return nil
	}
}

// ServeGRPCAndHTTPContext joins two listeners and shuts both down if either stops or fails.
func ServeGRPCAndHTTPContext(ctx context.Context, address string, grpcServer *grpc.Server, httpConfig HTTPContextConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 2)
	go func() { done <- ServeGRPCContext(ctx, address, grpcServer) }()
	go func() { done <- ServeHTTPContext(ctx, httpConfig) }()
	first := <-done
	cancel()
	return errors.Join(first, <-done)
}
