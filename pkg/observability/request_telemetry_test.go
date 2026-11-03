package observability

import (
	"context"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func TestRequestTelemetryOwnsRegistryAndCompletesMetrics(t *testing.T) {
	for range 2 {
		telemetry, err := NewRequestTelemetry(t.Context(), RequestTelemetryConfig{ServiceName: "service-b"})
		if err != nil {
			t.Fatal(err)
		}
		ctx, observation := telemetry.StartRequest(t.Context(), "http", "unmatched", trace.SpanKindServer)
		if !trace.SpanContextFromContext(ctx).IsValid() {
			t.Fatal("missing trace context")
		}
		observation.End("GET /orders/{id}", "500", "failure")
		response := httptest.NewRecorder()
		telemetry.MetricsHandler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
		body := response.Body.String()
		if !strings.Contains(body, `service_requests_total{operation="GET /orders/{id}",outcome="failure",protocol="http",service="service-b",status="500"} 1`) || !strings.Contains(body, `service_requests_in_flight{protocol="http",service="service-b"} 0`) {
			t.Fatalf("missing completed request metrics: %s", body)
		}
		if err := telemetry.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

type testTraceCollector struct {
	collector.UnimplementedTraceServiceServer
	exports chan *collector.ExportTraceServiceRequest
}

func (c *testTraceCollector) Export(_ context.Context, request *collector.ExportTraceServiceRequest) (*collector.ExportTraceServiceResponse, error) {
	c.exports <- request
	return &collector.ExportTraceServiceResponse{}, nil
}

func TestRequestTracingFlushesToConfiguredLoopbackCollector(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	collectorServer := &testTraceCollector{exports: make(chan *collector.ExportTraceServiceRequest, 4)}
	collector.RegisterTraceServiceServer(server, collectorServer)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	telemetry, err := NewRequestTelemetry(t.Context(), RequestTelemetryConfig{ServiceName: "service-b", TraceEndpoint: "http://" + listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetry.Close(ctx)
	})
	traceID, err := trace.TraceIDFromHex("08095642d644499bae01064e9be48b27")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("08095642d644499b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithRemoteSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true}))
	_, observation := telemetry.StartRequest(ctx, "http", "GET /orders", trace.SpanKindServer)
	observation.End("GET /orders", "200", "success")
	shutdown, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := telemetry.Close(shutdown); err != nil {
		t.Fatal(err)
	}
	select {
	case exported := <-collectorServer.exports:
		if len(exported.ResourceSpans) != 1 || len(exported.ResourceSpans[0].ScopeSpans) != 1 || len(exported.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatal("missing exported request span")
		}
		span := exported.ResourceSpans[0].ScopeSpans[0].Spans[0]
		if span.Name != "GET /orders" {
			t.Fatalf("span operation = %q", span.Name)
		}
	case <-shutdown.Done():
		t.Fatal("tracing did not flush on shutdown")
	}
}

func TestRequestTelemetryRejectsUnsafeCollectorEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://collector:4317", "https://user:secret@example.test:4317", "https://example.test:4317?token=secret"} {
		if telemetry, err := NewRequestTelemetry(t.Context(), RequestTelemetryConfig{ServiceName: "test", TraceEndpoint: endpoint}); err == nil {
			_ = telemetry.Close(t.Context())
			t.Error("unsafe endpoint accepted")
		}
	}
}
