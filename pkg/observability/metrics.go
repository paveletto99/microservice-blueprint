// Package server provides an opinionated http server.
package observability

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"

	// "os"
	"strings"
	"time"

	"github.com/paveletto99/go-pobo/pkg/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

type MetricsDoneFunc func() error

// ServeMetricsIfPrometheus serves the opentelemetry metrics at /metrics when
// OBSERVABILITY_EXPORTER set to "prometheus".
func ServeMetricsIfPrometheus(ctx context.Context) (MetricsDoneFunc, error) {
	logger := logging.FromContext(ctx)
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
		http.Handle("/metrics", exporter)

		srv := &http.Server{
			Addr:              ":" + metricsPort,
			ReadHeaderTimeout: 10 * time.Second,
			Handler:           &r,
		}

		recordMetrics(ctx)

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

const meterName = "go.opentelemetry.io/otel/example/prometheus"

func recordMetrics(ctx context.Context) {
	logger := logging.FromContext(ctx)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// The exporter embeds a default OpenTelemetry Reader and
	// implements prometheus.Collector, allowing it to be used as
	// both a Reader and Collector.
	exporter, err := prometheus.New()
	if err != nil {
		logger.Fatal(err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	meter := provider.Meter(meterName)

	// Start the prometheus HTTP server and pass the exporter Collector to it
	opt := api.WithAttributes(
		attribute.Key("A").String("B"),
		attribute.Key("C").String("D"),
	)

	// This is the equivalent of prometheus.NewCounterVec
	counter, err := meter.Float64Counter("foo", api.WithDescription("a simple counter"))
	if err != nil {
		logger.Fatal(err)
	}
	counter.Add(ctx, 5, opt)

	gauge, err := meter.Float64ObservableGauge("bar", api.WithDescription("a fun little gauge"))
	if err != nil {
		logger.Fatal(err)
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
		n := -10. + rng.Float64()*(90.) // [-10, 100)
		o.ObserveFloat64(gauge, n, opt)
		return nil
	}, gauge)
	if err != nil {
		logger.Fatal(err)
	}

	// This is the equivalent of prometheus.NewHistogramVec
	histogram, err := meter.Float64Histogram(
		"baz",
		api.WithDescription("a histogram with custom buckets and rename"),
		api.WithExplicitBucketBoundaries(64, 128, 256, 512, 1024, 2048, 4096),
	)
	if err != nil {
		logger.Fatal(err)
	}
	histogram.Record(ctx, 136, opt)
	histogram.Record(ctx, 64, opt)
	histogram.Record(ctx, 701, opt)
	histogram.Record(ctx, 830, opt)
}
