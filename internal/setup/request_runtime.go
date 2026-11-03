package setup

import (
	"context"
	"os"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/pkg/observability"
)

// RequestRuntime owns process-scoped request policy and telemetry, not business dependencies.
type RequestRuntime struct {
	Stack     *middleware.RequestStack
	telemetry *observability.RequestTelemetry
}

// NewRequestRuntime loads typed policy once and creates an isolated registry and optional exporter.
func NewRequestRuntime(ctx context.Context, service string) (*RequestRuntime, error) {
	policy, err := middleware.RequestPolicyFromEnv()
	if err != nil {
		return nil, err
	}
	telemetry, err := observability.NewRequestTelemetry(ctx, observability.RequestTelemetryConfig{ServiceName: service, TraceEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")})
	if err != nil {
		return nil, err
	}
	stack, err := middleware.NewRequestStack(policy, telemetry)
	if err != nil {
		_ = telemetry.Close(ctx)
		return nil, err
	}
	return &RequestRuntime{Stack: stack, telemetry: telemetry}, nil
}

// Close flushes request telemetry after transport shutdown, bounded to five seconds.
func (r *RequestRuntime) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.telemetry.Close(ctx)
}
