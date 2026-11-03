package observability

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DependencyReadinessMetric checks a fixed dependency with a one-second scrape budget.
// Probes must honor context cancellation; errors and resource addresses are never exposed.
func DependencyReadinessMetric(name string, probe func(context.Context) error) (prometheus.Collector, error) {
	if probe == nil || (name != "oauth" && name != "jwks" && name != "database" && name != "payment") {
		return nil, errors.New("dependency metrics: unsupported dependency or missing probe")
	}
	return prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "service_dependency_ready", Help: "Whether a dependency is ready to serve work.", ConstLabels: prometheus.Labels{"dependency": name}}, func() (ready float64) {
		// Collector callbacks execute outside HTTP recovery. A broken dependency
		// probe must report unready rather than panic on a collector goroutine.
		defer func() {
			if recover() != nil {
				ready = 0
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if probe(ctx) != nil {
			return 0
		}
		return 1
	}), nil
}
