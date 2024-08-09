// Package server provides an opinionated http server.
package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/paveletto99/go-pobo/pkg/logging"
	// "contrib.go.opencensus.io/exporter/prometheus"
	// "github.com/gorilla/mux"
)

type MetricsDoneFunc func() error

// ServeMetricsIfPrometheus serves the opencensus metrics at /metrics when
// OBSERVABILITY_EXPORTER set to "prometheus".
func ServeMetricsIfPrometheus(ctx context.Context) (MetricsDoneFunc, error) {
	logger := logging.FromContext(ctx)

	exporter := os.Getenv("OBSERVABILITY_EXPORTER")
	if strings.EqualFold(exporter, "prometheus") {
		metricsPort := os.Getenv("METRICS_PORT")
		if metricsPort == "" {
			return nil, fmt.Errorf("OBSERVABILITY_EXPORTER set to 'prometheus' but no METRICS_PORT set")
		}

		exporter, err := prometheus.NewExporter(prometheus.Options{})
		if err != nil {
			return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
		}

		r := *http.ServeMux
		r.Handle("/metrics", exporter)
		srv := &http.Server{
			Addr:              ":" + metricsPort,
			ReadHeaderTimeout: 10 * time.Second,
			Handler:           r,
		}

		// Start the server in the background.
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Errorw("failed to serve prometheus metrics", "error", err)
				return
			}
		}()
		logger.Debugw("prometheus exporter is running", "port", metricsPort)

		// Create the shutdown closer.
		metricsDone := func() error {
			logger.Debugw("shutting down prometheus metrics exporter")

			shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("failed to shutdown prometheus metrics exporter: %w", err)
			}
			logger.Debugw("finished shutting down prometheus metrics exporter")

			return nil
		}

		return metricsDone, nil
	}

	return nil, nil
}
