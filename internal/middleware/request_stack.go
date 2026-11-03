package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/observability"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// RequestStack shares operational policy between HTTP middleware and unary gRPC interceptors.
// Construct once per process; never per request. TLS and business permissions remain separate.
type RequestStack struct {
	policy    RequestPolicy
	telemetry *observability.RequestTelemetry
	permits   chan struct{}
}

// NewRequestStack validates request budgets and uses an explicitly owned telemetry registry.
func NewRequestStack(policy RequestPolicy, telemetry *observability.RequestTelemetry) (*RequestStack, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if telemetry == nil {
		return nil, errors.New("request stack: telemetry is required")
	}
	telemetry.SetBusinessCapacity(policy.MaxInFlight)
	return &RequestStack{policy: policy, telemetry: telemetry, permits: make(chan struct{}, policy.MaxInFlight)}, nil
}

type requestStateKey struct{}
type requestIDKey struct{}
type requestState struct {
	operation   string
	panicked    bool
	operational bool
}

func requestStateFrom(ctx context.Context) *requestState {
	state, _ := ctx.Value(requestStateKey{}).(*requestState)
	return state
}

func requestLoggerContext(ctx context.Context) context.Context {
	span := trace.SpanContextFromContext(ctx)
	id := uuid.NewString()
	logger := logging.FromContext(ctx).With("request_id", id, "trace_id", span.TraceID().String())
	ctx = context.WithValue(ctx, requestIDKey{}, id)
	return logging.WithLogger(ctx, logger)
}

// HTTP installs context, observation and recovery once around the composed router.
// Route wrappers below decide business budgets; no lifecycle function installs middleware.
func (s *RequestStack) HTTP(next http.Handler) http.Handler {
	recovered := Recovery()(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.TracePropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, observation := s.telemetry.StartRequest(ctx, "http", "unmatched", trace.SpanKindServer)
		ctx = requestLoggerContext(ctx)
		state := &requestState{operation: "unmatched"}
		ctx = context.WithValue(ctx, requestStateKey{}, state)
		response := &responseState{ResponseWriter: w}
		// The response ID is locally generated and is never an authentication input.
		response.Header().Set("X-Request-ID", ctx.Value(requestIDKey{}).(string))
		defer func() {
			p := recover()
			code := response.status
			if code == 0 && p == nil {
				code = http.StatusOK
			}
			outcome := httpOutcome(code)
			if p != nil || response.writeFailed {
				outcome = "aborted"
			}
			observation.End(state.operation, strconv.Itoa(code), outcome)
			logger := logging.FromContext(ctx)
			if (outcome == "failure" || response.writeFailed) && !state.panicked && !state.operational {
				logger.Errorw("http request failed", "operation", state.operation, "status", code)
			} else {
				logger.Infow("http request completed", "operation", state.operation, "status", code, "outcome", outcome)
			}
			if p != nil {
				panic(p)
			}
		}()
		recovered.ServeHTTP(response, r.WithContext(ctx))
	})
}

// OperationalHTTP labels a public health/metrics route without business capacity or auth.
func (s *RequestStack) OperationalHTTP(pattern string, next http.Handler) http.Handler {
	return labelHTTPRoute(pattern, true, next)
}

func labelHTTPRoute(pattern string, operational bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if state := requestStateFrom(r.Context()); state != nil {
			state.operation = pattern
			state.operational = operational
		}
		next.ServeHTTP(w, r)
	})
}

// BusinessHTTP applies a cooperative deadline and non-queuing capacity before business work.
// A handler must honor context cancellation; this never creates a detached timeout goroutine.
func (s *RequestStack) BusinessHTTP(pattern string, next http.Handler) http.Handler {
	return labelHTTPRoute(pattern, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.policy.Timeout)
		defer cancel()
		if ctx.Err() != nil {
			code := http.StatusRequestTimeout
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = http.StatusGatewayTimeout
			}
			http.Error(w, "request canceled or expired", code)
			return
		}
		select {
		case s.permits <- struct{}{}:
			s.telemetry.BusinessStarted()
			defer func() { s.telemetry.BusinessCompleted(); <-s.permits }()
		default:
			s.telemetry.CapacityRejected("http")
			w.Header().Set("Retry-After", "1")
			http.Error(w, "service capacity exhausted", http.StatusServiceUnavailable)
			return
		}
		deadline, _ := ctx.Deadline()
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(w, "request transport unavailable", http.StatusInternalServerError)
			return
		}
		defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
		next.ServeHTTP(w, r.WithContext(ctx))
		if response, ok := w.(*responseState); ok && response.status == 0 && ctx.Err() != nil {
			code := http.StatusRequestTimeout
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = http.StatusGatewayTimeout
			}
			http.Error(w, "request deadline or cancellation", code)
		}
	}))
}

// MetricsHandler exposes the stack's process-owned registry; mount it operationally.
func (s *RequestStack) MetricsHandler() http.Handler { return s.telemetry.MetricsHandler() }

// RegisterMetrics registers feature metrics without opening a listener or global registry.
func (s *RequestStack) RegisterMetrics(collectors ...prometheus.Collector) error {
	return s.telemetry.RegisterMetrics(collectors...)
}

// RegisterDependency exposes a bounded readiness probe without using transport status as health.
func (s *RequestStack) RegisterDependency(name string, probe func(context.Context) error) error {
	metric, err := observability.DependencyReadinessMetric(name, probe)
	if err != nil {
		return err
	}
	return s.RegisterMetrics(metric)
}

func httpOutcome(code int) string {
	if code >= 500 {
		return "failure"
	}
	if code >= 400 {
		return "rejected"
	}
	return "success"
}
