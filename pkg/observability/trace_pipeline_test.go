package observability

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type blockedTraceExporter struct {
	entered, release chan struct{}
	once             sync.Once
	err              error
}

func TestCanceledFlushesCannotConsumeSpanQueueSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		metrics := newTracePipelineMetrics(prometheus.Labels{"service": "test"})
		exporter := &blockedTraceExporter{entered: make(chan struct{}), release: make(chan struct{})}
		processor := newMonitoredTraceProcessor(exporter, metrics, 2, 1)
		provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(processor))
		var release sync.Once
		defer func() { release.Do(func() { close(exporter.release) }); _ = provider.Shutdown(context.Background()) }()
		tracer := provider.Tracer("test")
		_, first := tracer.Start(t.Context(), "first")
		first.End()
		<-exporter.entered
		for range 3 {
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- provider.ForceFlush(ctx) }()
			synctest.Wait() // Ensure the flush is waiting before cancellation, without sleeps.
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("flush cancellation = %v", err)
			}
		}
		_, second := tracer.Start(t.Context(), "second")
		second.End()
		release.Do(func() { close(exporter.release) })
		if err := provider.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		if metrics.pending.Load() != 0 || counterValue(t, metrics.exported) != 2 {
			t.Fatal("canceled flushes consumed span queue slots")
		}
	})
}

func (e *blockedTraceExporter) ExportSpans(ctx context.Context, _ []sdktrace.ReadOnlySpan) error {
	e.once.Do(func() { close(e.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.release:
		return e.err
	}
}
func (*blockedTraceExporter) Shutdown(context.Context) error { return nil }

func counterValue(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatal(err)
	}
	return metric.GetCounter().GetValue()
}

func TestTraceQueueDropsWithoutBlockingApplication(t *testing.T) {
	metrics := newTracePipelineMetrics(prometheus.Labels{"service": "test"})
	exporter := &blockedTraceExporter{entered: make(chan struct{}), release: make(chan struct{})}
	processor := newMonitoredTraceProcessor(exporter, metrics, 1, 1)
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(processor))
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(exporter.release) })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})
	tracer := provider.Tracer("test")
	_, first := tracer.Start(t.Context(), "first")
	first.End()
	select {
	case <-exporter.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("export did not start")
	}
	_, second := tracer.Start(t.Context(), "second")
	second.End()
	for range 100 {
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := provider.ForceFlush(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled flush = %v", err)
		}
	}
	_, third := tracer.Start(t.Context(), "third")
	third.End()
	if metrics.pending.Load() != 1 || counterValue(t, metrics.dropped.WithLabelValues("queue_full")) != 1 {
		t.Fatal("bounded admission did not report the dropped span")
	}
	release.Do(func() { close(exporter.release) })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if metrics.pending.Load() != 0 || counterValue(t, metrics.exported) != 2 {
		t.Fatal("admitted spans did not drain exactly once")
	}
}

func TestTraceExportFailuresAreCountedAndSanitized(t *testing.T) {
	metrics := newTracePipelineMetrics(prometheus.Labels{"service": "test"})
	release := make(chan struct{})
	close(release)
	exporter := &blockedTraceExporter{entered: make(chan struct{}), release: release, err: errors.New("sensitive collector response")}
	wrapped := &monitoredTraceExporter{next: exporter, metrics: metrics}
	metrics.pending.Store(1)
	err := wrapped.ExportSpans(t.Context(), []sdktrace.ReadOnlySpan{nil})
	if err == nil || strings.Contains(err.Error(), "sensitive") || counterValue(t, metrics.failed) != 1 || counterValue(t, metrics.batches.WithLabelValues("failure")) != 1 || metrics.pending.Load() != 0 {
		t.Fatal("export failure was not counted or sanitized")
	}
}

func BenchmarkRequestObservation(b *testing.B) {
	telemetry, err := NewRequestTelemetry(b.Context(), RequestTelemetryConfig{ServiceName: "benchmark"})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = telemetry.Close(context.Background()) })
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, observation := telemetry.StartRequest(b.Context(), "http", "GET /orders", trace.SpanKindServer)
		observation.End("GET /orders", "200", "success")
	}
}
