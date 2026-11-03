package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestContextServersJoinOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ServeHTTPContext(ctx, HTTPContextConfig{Address: "127.0.0.1:0", Handler: http.NotFoundHandler()}); err != nil {
		t.Fatal(err)
	}
	if err := ServeGRPCContext(ctx, "127.0.0.1:0", grpc.NewServer()); err != nil {
		t.Fatal(err)
	}
}

func TestDualTransportStartupFailureStopsOtherListener(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	done := make(chan error, 1)
	go func() {
		done <- ServeGRPCAndHTTPContext(t.Context(), occupied.Addr().String(), grpc.NewServer(), HTTPContextConfig{Address: "127.0.0.1:0", Handler: http.NotFoundHandler()})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("listener failure was ignored")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("transport goroutines did not join")
	}
}
