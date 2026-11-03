package observability

import (
	"errors"
	"testing"
)

func TestLegacyMetricsAPIsRejectImplicitListeners(t *testing.T) {
	if closer, err := ServeMetricsIfPrometheus(t.Context()); closer != nil || !errors.Is(err, ErrLegacyMetricsRetired) {
		t.Fatal("implicit metrics listener was not retired")
	}
	if factory, err := NewPrometheusMetricsFactory(PrometheusConfig{Port: "2223"}); factory != nil || !errors.Is(err, ErrLegacyMetricsRetired) {
		t.Fatal("legacy factory was not retired")
	}
	// Direct zero-value construction remains harmless and source-compatible.
	metric := (PrometheusMetricsFactory{}).Create("legacy")
	metric.Fire("ignored")
	metric.ResponseStatus("ignored", 200)
}
