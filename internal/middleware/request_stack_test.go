package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paveletto99/microservice-blueprint/pkg/observability"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func testRequestStack(t *testing.T, limit int) (*RequestStack, *observability.RequestTelemetry) {
	t.Helper()
	telemetry, err := observability.NewRequestTelemetry(t.Context(), observability.RequestTelemetryConfig{ServiceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { telemetry.Close(context.Background()) })
	stack, err := NewRequestStack(RequestPolicy{Timeout: time.Second, MaxInFlight: limit}, telemetry)
	if err != nil {
		t.Fatal(err)
	}
	return stack, telemetry
}

func TestTraceContextLinksHTTPAndGRPC(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	serverStack, _ := testRequestStack(t, 1)
	handler := stack.HTTP(stack.BusinessHTTP("GET /orders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parent := trace.SpanContextFromContext(r.Context()).TraceID()
		err := stack.UnaryClientInterceptor()(r.Context(), "/payment.v1.PaymentService/CreatePayment", nil, nil, nil, func(ctx context.Context, method string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			md, _ := metadata.FromOutgoingContext(ctx)
			incoming := metadata.NewIncomingContext(context.Background(), md)
			_, err := serverStack.UnaryServerInterceptor()(incoming, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ any) (any, error) {
				if trace.SpanContextFromContext(ctx).TraceID() != parent {
					t.Error("HTTP-to-gRPC trace context was not preserved")
				}
				return nil, nil
			})
			return err
		})
		if err != nil {
			t.Error(err)
		}
		w.WriteHeader(200)
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/orders", nil))
	if response.Code != 200 {
		t.Fatalf("trace flow response = %d", response.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPTransportPropagatesTraceWithoutMutatingRequests(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	transport := stack.HTTPTransport("service-b", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("traceparent") == "" {
			t.Error("missing outgoing trace context")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))
	request := httptest.NewRequest("GET", "http://example.test/orders", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if request.Header.Get("traceparent") != "" {
		t.Fatal("transport mutated caller headers")
	}
}

func TestGRPCOperationalCapacityAndMetadataIsolation(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	interceptor := stack.UnaryServerInterceptor()
	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/payment.v1.PaymentService/CreatePayment"}, func(ctx context.Context, _ any) (any, error) {
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/payment.v1.PaymentService/CreatePayment"}, func(context.Context, any) (any, error) { t.Error("overload invoked business handler"); return nil, nil })
		if status.Code(err) != codes.ResourceExhausted {
			t.Error("missing gRPC overload rejection")
		}
		return interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, func(context.Context, any) (any, error) { return nil, nil })
	})
	if err != nil {
		t.Fatal("health was blocked by business capacity")
	}
	md := metadata.Pairs("authorization", "Bearer private")
	ctx := metadata.NewOutgoingContext(t.Context(), md)
	err = stack.UnaryClientInterceptor()(ctx, "/payment.v1.PaymentService/CreatePayment", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		outgoing, _ := metadata.FromOutgoingContext(ctx)
		if len(outgoing.Get("traceparent")) != 1 || outgoing.Get("authorization")[0] != "Bearer private" {
			t.Error("trace injection lost metadata")
		}
		return nil
	})
	if err != nil || len(md.Get("traceparent")) != 0 {
		t.Fatal("interceptor mutated caller metadata")
	}
}

func TestExpiredHTTPRequestsDoNotInvokeBusinessHandler(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	handler := stack.HTTP(stack.BusinessHTTP("GET /orders", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("expired request invoked business handler") })))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, "GET", "/orders", nil))
	if response.Code != 504 {
		t.Fatalf("expired request status = %d", response.Code)
	}
}

func TestHTTPStackDeadlineCapacityAndOperationalIsolation(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() { close(release); <-done }()
	business := stack.BusinessHTTP("GET /orders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > time.Second {
			t.Error("missing bounded request deadline")
		}
		close(entered)
		<-release
		w.WriteHeader(204)
	}))
	mux := http.NewServeMux()
	mux.Handle("GET /orders", business)
	mux.Handle("GET /healthz", stack.OperationalHTTP("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })))
	handler := stack.HTTP(mux)
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/orders", nil))
	}()
	<-entered
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest("GET", "/orders", nil))
	if rejected.Code != 503 || rejected.Header().Get("Retry-After") == "" {
		t.Fatalf("overload response = %d", rejected.Code)
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest("GET", "/healthz", nil))
	if health.Code != 200 {
		t.Fatal("health was blocked by business capacity")
	}
}

func TestHTTPStackRecoveryAndMetrics(t *testing.T) {
	stack, telemetry := testRequestStack(t, 1)
	handler := stack.HTTP(stack.BusinessHTTP("GET /orders/{id}", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("sensitive panic value") })))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/orders/private-id", nil))
	if w.Code != 500 || w.Header().Get("X-Request-ID") == "" || strings.Contains(w.Body.String(), "sensitive") {
		t.Fatalf("panic response = %d %s", w.Code, w.Body.String())
	}
	metrics := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `operation="GET /orders/{id}"`) || strings.Contains(metrics.Body.String(), "private-id") {
		t.Fatal("request metric lost the route template or exposed an ID")
	}
}

func TestRecoveryAbortsCommittedResponseAndPreservesAbortHandler(t *testing.T) {
	for _, panicValue := range []any{"sensitive", http.ErrAbortHandler} {
		func() {
			defer func() {
				if p := recover(); p != http.ErrAbortHandler {
					t.Errorf("panic = %v, want ErrAbortHandler", p)
				}
			}()
			Recovery()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				_, _ = io.WriteString(w, "partial")
				panic(panicValue)
			})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
			t.Error("committed response panic was swallowed")
		}()
	}
}

func TestGRPCStackRecoversAndPreservesCallerDeadline(t *testing.T) {
	stack, _ := testRequestStack(t, 1)
	interceptor := stack.UnaryServerInterceptor()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/payment.v1.PaymentService/CreatePayment"}, func(ctx context.Context, _ any) (any, error) {
		deadline, _ := ctx.Deadline()
		if time.Until(deadline) > 100*time.Millisecond {
			t.Error("caller deadline was extended")
		}
		panic("private detail")
	})
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "private detail") {
		t.Fatalf("panic error = %v", err)
	}
	_, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/payment.v1.PaymentService/CreatePayment"}, func(context.Context, any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("panic leaked the capacity permit: %v", err)
	}
}
