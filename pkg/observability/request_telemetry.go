package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// RequestTelemetryConfig configures a process-owned registry and optional OTLP tracing.
// An empty TraceEndpoint disables export; no collector is started by this module.
type RequestTelemetryConfig struct {
	ServiceName    string
	TraceEndpoint  string
	TraceQueueSize int
	TraceBatchSize int
}

// RequestTelemetry owns request metrics and tracing without global registration.
type RequestTelemetry struct {
	registry         *prometheus.Registry
	requests         *prometheus.CounterVec
	duration         *prometheus.HistogramVec
	inFlight         *prometheus.GaugeVec
	provider         *sdktrace.TracerProvider
	tracer           trace.Tracer
	businessInFlight prometheus.Gauge
	businessCapacity prometheus.Gauge
	capacityRejected *prometheus.CounterVec
	traceMetrics     *tracePipelineMetrics
}

// NewRequestTelemetry accepts HTTPS OTLP endpoints, or numeric loopback HTTP collectors.
func NewRequestTelemetry(ctx context.Context, config RequestTelemetryConfig) (*RequestTelemetry, error) {
	if config.ServiceName == "" || len(config.ServiceName) > 64 || strings.ContainsAny(config.ServiceName, " \r\n\t") {
		return nil, errors.New("request telemetry: invalid service name")
	}
	if config.TraceQueueSize == 0 {
		config.TraceQueueSize = 512
	}
	if config.TraceBatchSize == 0 {
		config.TraceBatchSize = min(128, config.TraceQueueSize)
	}
	if config.TraceQueueSize < 1 || config.TraceQueueSize > 8192 || config.TraceBatchSize < 1 || config.TraceBatchSize > config.TraceQueueSize {
		return nil, errors.New("request telemetry: invalid trace queue bounds")
	}
	labels := prometheus.Labels{"service": config.ServiceName}
	traceMetrics := newTracePipelineMetrics(labels)
	options := []sdktrace.TracerProviderOption{sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))), sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", config.ServiceName)))}
	if config.TraceEndpoint != "" {
		u, err := url.Parse(config.TraceEndpoint)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())) {
			return nil, errors.New("request telemetry: HTTPS or loopback HTTP collector required")
		}
		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(config.TraceEndpoint), otlptracegrpc.WithTimeout(2*time.Second))
		if err != nil {
			return nil, errors.New("request telemetry: tracing exporter initialization failed")
		}
		options = append(options, sdktrace.WithSpanProcessor(newMonitoredTraceProcessor(exporter, traceMetrics, config.TraceQueueSize, config.TraceBatchSize)))
	}
	provider := sdktrace.NewTracerProvider(options...)
	t := &RequestTelemetry{
		registry: prometheus.NewRegistry(), provider: provider, tracer: provider.Tracer("microservice-blueprint/requests"), traceMetrics: traceMetrics,
		requests:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_requests_total", Help: "Completed transport requests.", ConstLabels: labels}, []string{"protocol", "operation", "status", "outcome"}),
		duration:         prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "service_request_duration_seconds", Help: "Transport request duration in seconds.", ConstLabels: labels}, []string{"protocol", "operation"}),
		inFlight:         prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "service_requests_in_flight", Help: "Active transport requests.", ConstLabels: labels}, []string{"protocol"}),
		businessInFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_business_requests_in_flight", Help: "Business requests holding a capacity permit.", ConstLabels: labels}),
		businessCapacity: prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_business_request_capacity", Help: "Configured business request capacity.", ConstLabels: labels}),
		capacityRejected: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_capacity_rejections_total", Help: "Requests rejected by business capacity.", ConstLabels: labels}, []string{"protocol"}),
	}
	t.registry.MustRegister(t.requests, t.duration, t.inFlight, t.businessInFlight, t.businessCapacity, t.capacityRejected, prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	t.registry.MustRegister(traceMetrics.collectors()...)
	return t, nil
}

// RequestObservation finishes one request; operation must be a route template or fixed RPC method.
type RequestObservation struct {
	telemetry *RequestTelemetry
	span      trace.Span
	protocol  string
	started   time.Time
	end       sync.Once
}

// StartRequest starts a transport span and counts in-flight work; call End exactly once.
func (t *RequestTelemetry) StartRequest(ctx context.Context, protocol, operation string, kind trace.SpanKind) (context.Context, *RequestObservation) {
	ctx, span := t.tracer.Start(ctx, operation, trace.WithSpanKind(kind), trace.WithAttributes(attribute.String("transport.protocol", protocol)))
	t.inFlight.WithLabelValues(protocol).Inc()
	return ctx, &RequestObservation{telemetry: t, span: span, protocol: protocol, started: time.Now()}
}

// End records only bounded status/outcome labels, never headers, payloads, IDs or error strings.
func (o *RequestObservation) End(operation, status, outcome string) {
	o.end.Do(func() {
		o.span.SetName(operation)
		o.span.SetAttributes(attribute.String("request.operation", operation), attribute.String("response.status", status), attribute.String("request.outcome", outcome))
		if outcome == "failure" || outcome == "aborted" {
			o.span.SetStatus(codes.Error, "request failed")
		}
		o.span.End()
		o.telemetry.requests.WithLabelValues(o.protocol, operation, status, outcome).Inc()
		o.telemetry.duration.WithLabelValues(o.protocol, operation).Observe(time.Since(o.started).Seconds())
		o.telemetry.inFlight.WithLabelValues(o.protocol).Dec()
	})
}

// MetricsHandler exposes this process's request metrics on an operational route.
func (t *RequestTelemetry) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{})
}

// RegisterMetrics installs feature-owned collectors into the same process registry.
func (t *RequestTelemetry) RegisterMetrics(collectors ...prometheus.Collector) error {
	for _, collector := range collectors {
		if err := t.registry.Register(collector); err != nil {
			return errors.New("request telemetry: duplicate or invalid feature collector")
		}
	}
	return nil
}

// SetBusinessCapacity reports the validated process policy, not a per-request limit.
func (t *RequestTelemetry) SetBusinessCapacity(capacity int) {
	t.businessCapacity.Set(float64(capacity))
}

// BusinessStarted counts an acquired permit; release it with BusinessCompleted even after panic.
func (t *RequestTelemetry) BusinessStarted() { t.businessInFlight.Inc() }

// BusinessCompleted releases one observed capacity permit.
func (t *RequestTelemetry) BusinessCompleted() { t.businessInFlight.Dec() }

// CapacityRejected records a non-queuing rejection for a fixed transport name.
func (t *RequestTelemetry) CapacityRejected(protocol string) {
	t.capacityRejected.WithLabelValues(protocol).Inc()
}

// Close flushes tracing using the caller's shutdown deadline.
func (t *RequestTelemetry) Close(ctx context.Context) error { return t.provider.Shutdown(ctx) }

// TracePropagator propagates W3C trace context only; baggage is deliberately not accepted.
func TracePropagator() propagation.TextMapPropagator { return propagation.TraceContext{} }
