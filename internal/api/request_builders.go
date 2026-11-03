package api

import (
	"context"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/pkg/observability"
)

// Compatibility constructors use isolated defaults, never environment-dependent exporters.
func defaultServiceRequests(service string) (*middleware.RequestStack, error) {
	telemetry, err := observability.NewRequestTelemetry(context.Background(), observability.RequestTelemetryConfig{ServiceName: service})
	if err != nil {
		return nil, err
	}
	return middleware.NewRequestStack(middleware.DefaultRequestPolicy(), telemetry)
}
