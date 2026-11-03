package observability

// PrometheusConfig is retained only for source compatibility.
// Deprecated: use RequestTelemetryConfig and explicitly compose MetricsHandler.
type PrometheusConfig struct{ Host, Port, ScrapePath, MetricName string }

// PrometheusMetrics is a retired, inert compatibility type.
// Deprecated: use RequestTelemetry.StartRequest and RequestObservation.End.
type PrometheusMetrics struct{}

// Fire is inert; legacy timing and global registration have been retired.
func (*PrometheusMetrics) Fire(string) {}

// ResponseStatus is inert; use the canonical request observation instead.
func (*PrometheusMetrics) ResponseStatus(string, int) {}

// PrometheusMetricsFactory is retained only for source compatibility.
// Deprecated: use NewRequestTelemetry; this type never starts a listener.
type PrometheusMetricsFactory struct{}

// NewPrometheusMetricsFactory rejects the retired implicit-listener API.
// Deprecated: create RequestTelemetry and explicitly mount its MetricsHandler.
func NewPrometheusMetricsFactory(PrometheusConfig) (MetricsFactory, error) {
	return nil, ErrLegacyMetricsRetired
}

// Create is inert for callers constructing a retired factory directly.
func (PrometheusMetricsFactory) Create(string) Metrics { return &PrometheusMetrics{} }
