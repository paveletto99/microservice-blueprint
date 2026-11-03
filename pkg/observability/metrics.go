package observability

import (
	"context"
	"errors"
)

// ErrLegacyMetricsRetired rejects APIs that previously opened unmanaged/global listeners.
var ErrLegacyMetricsRetired = errors.New("observability: legacy metrics API retired; use RequestTelemetry with an explicit metrics route")

// Metrics is retained only for source compatibility with retired factories.
// Deprecated: use RequestObservation for request timing and feature-owned collectors otherwise.
type Metrics interface {
	Fire(string)
	ResponseStatus(string, int)
}

// MetricsFactory is retained only for source compatibility.
// Deprecated: use NewRequestTelemetry.
type MetricsFactory interface{ Create(string) Metrics }

// MetricsDoneFunc is retained only for source compatibility.
// Deprecated: metrics listeners belong to the explicit transport lifecycle.
type MetricsDoneFunc func() error

// ServeMetricsIfPrometheus no longer opens the hard-coded global registry listener.
// Deprecated: explicitly mount RequestTelemetry.MetricsHandler on an owned listener.
func ServeMetricsIfPrometheus(context.Context) (MetricsDoneFunc, error) {
	return nil, ErrLegacyMetricsRetired
}
