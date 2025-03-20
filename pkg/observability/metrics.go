// Package server provides an opinionated http server.
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics interface {
	Fire(result string)
	ResponseStatus(prefix string, status int)
}

type MetricsFactory interface {
	Create(eventName string) Metrics
}

type MetricsDoneFunc func() error

// ServeMetricsIfPrometheus serves the opentelemetry metrics at /metrics when
// OBSERVABILITY_EXPORTER set to "prometheus".
func ServeMetricsIfPrometheus(ctx context.Context) (MetricsDoneFunc, error) {
	// exporter := os.Getenv("OBSERVABILITY_EXPORTER")

	exporter := "prometheus"
	if strings.EqualFold(exporter, "prometheus") {
		// metricsPort := os.Getenv("METRICS_PORT")
		metricsPort := "2223"
		if metricsPort == "" {
			return nil, fmt.Errorf("OBSERVABILITY_EXPORTER set to 'prometheus' but no METRICS_PORT set")
		}

		exporter := promhttp.Handler()

		r := *http.NewServeMux()
		r.Handle("/metrics", exporter)

		srv := &http.Server{
			Addr:              ":" + metricsPort,
			ReadHeaderTimeout: 10 * time.Second,
			Handler:           &r,
		}

		// Start the server in the background.
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("failed to serve prometheus metrics", "error", err)
				return
			}
		}()
		slog.Debug("prometheus exporter is running", "port", metricsPort)

		// Create the shutdown closer.
		metricsDone := func() error {
			slog.Debug("shutting down prometheus metrics exporter")

			shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("failed to shutdown prometheus metrics exporter: %w", err)
			}
			slog.Debug("finished shutting down prometheus metrics exporter")

			return nil
		}

		return metricsDone, nil
	}

	return nil, nil
}
