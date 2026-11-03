package observability

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type tracePipelineMetrics struct {
	pending                                   atomic.Int64
	admitted, completed, failures             atomic.Int64
	changeMu                                  sync.Mutex
	changed                                   chan struct{}
	enabled, capacity, exporting, lastSuccess prometheus.Gauge
	batches                                   *prometheus.CounterVec
	exported, failed, shutdownFailures        prometheus.Counter
	dropped                                   *prometheus.CounterVec
	queue                                     prometheus.GaugeFunc
}

func newTracePipelineMetrics(labels prometheus.Labels) *tracePipelineMetrics {
	m := &tracePipelineMetrics{
		changed:          make(chan struct{}),
		enabled:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_trace_export_enabled", Help: "Whether trace export is configured.", ConstLabels: labels}),
		capacity:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_trace_queue_capacity", Help: "Maximum spans awaiting an export attempt.", ConstLabels: labels}),
		exporting:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_trace_export_in_flight", Help: "Active trace export batches.", ConstLabels: labels}),
		lastSuccess:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_trace_export_last_success_timestamp_seconds", Help: "Last successful trace export timestamp.", ConstLabels: labels}),
		batches:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_trace_export_batches_total", Help: "Trace export batch results after exporter retries.", ConstLabels: labels}, []string{"result"}),
		exported:         prometheus.NewCounter(prometheus.CounterOpts{Name: "service_trace_exported_spans_total", Help: "Successfully exported spans.", ConstLabels: labels}),
		failed:           prometheus.NewCounter(prometheus.CounterOpts{Name: "service_trace_export_failed_spans_total", Help: "Spans in failed export batches.", ConstLabels: labels}),
		shutdownFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "service_trace_shutdown_failures_total", Help: "Trace processor shutdown failures.", ConstLabels: labels}),
		dropped:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_trace_dropped_spans_total", Help: "Sampled spans rejected before export.", ConstLabels: labels}, []string{"reason"}),
	}
	m.queue = prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "service_trace_queue_pending", Help: "Admitted sampled spans not yet submitted to the exporter, including pending batches.", ConstLabels: labels}, func() float64 { return float64(m.pending.Load()) })
	m.batches.WithLabelValues("success")
	m.batches.WithLabelValues("failure")
	m.dropped.WithLabelValues("queue_full")
	m.dropped.WithLabelValues("shutdown")
	return m
}

func (m *tracePipelineMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.enabled, m.capacity, m.exporting, m.lastSuccess, m.batches, m.exported, m.failed, m.shutdownFailures, m.dropped, m.queue}
}

// monitoredTraceProcessor bounds admission before the SDK queue and keeps the SDK's batching,
// cancellation and shutdown behavior. ForceFlush waits for an admission snapshot rather than
// adding SDK flush markers that could fill the queue after repeated canceled flushes.
type monitoredTraceProcessor struct {
	processor sdktrace.SpanProcessor
	metrics   *tracePipelineMetrics
	capacity  int64
	gate      sync.RWMutex
	closing   bool
}

func newMonitoredTraceProcessor(exporter sdktrace.SpanExporter, metrics *tracePipelineMetrics, capacity, batch int) *monitoredTraceProcessor {
	p := &monitoredTraceProcessor{metrics: metrics, capacity: int64(capacity)}
	p.processor = sdktrace.NewBatchSpanProcessor(&monitoredTraceExporter{next: exporter, metrics: metrics}, sdktrace.WithMaxQueueSize(capacity), sdktrace.WithMaxExportBatchSize(batch), sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(2*time.Second))
	metrics.enabled.Set(1)
	metrics.capacity.Set(float64(capacity))
	return p
}

func (p *monitoredTraceProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	p.processor.OnStart(ctx, span)
}

func (p *monitoredTraceProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	if !span.SpanContext().IsSampled() {
		return
	}
	p.gate.RLock()
	defer p.gate.RUnlock()
	if p.closing {
		p.metrics.dropped.WithLabelValues("shutdown").Inc()
		return
	}
	for {
		pending := p.metrics.pending.Load()
		if pending >= p.capacity {
			p.metrics.dropped.WithLabelValues("queue_full").Inc()
			return
		}
		if p.metrics.pending.CompareAndSwap(pending, pending+1) {
			break
		}
	}
	p.metrics.admitted.Add(1)
	p.processor.OnEnd(span)
}

func (p *monitoredTraceProcessor) ForceFlush(ctx context.Context) error {
	// Hold the gate only long enough to put all prior admissions before later ones.
	// Network export remains asynchronous on the SDK's one-second schedule.
	p.gate.Lock()
	target, failures := p.metrics.admitted.Load(), p.metrics.failures.Load()
	p.gate.Unlock()
	for {
		p.metrics.changeMu.Lock()
		changed := p.metrics.changed
		p.metrics.changeMu.Unlock()
		if p.metrics.completed.Load() >= target {
			if p.metrics.failures.Load() > failures {
				return errors.New("trace processor: flush export failed")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (p *monitoredTraceProcessor) Shutdown(ctx context.Context) error {
	p.gate.Lock()
	p.closing = true
	p.gate.Unlock()
	err := p.processor.Shutdown(ctx)
	if err != nil {
		p.metrics.shutdownFailures.Inc()
	}
	return err
}

type monitoredTraceExporter struct {
	next    sdktrace.SpanExporter
	metrics *tracePipelineMetrics
}

func (e *monitoredTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.metrics.pending.Add(-int64(len(spans)))
	e.metrics.exporting.Inc()
	defer func() {
		e.metrics.exporting.Dec()
		e.metrics.completed.Add(int64(len(spans)))
		e.metrics.changeMu.Lock()
		close(e.metrics.changed)
		e.metrics.changed = make(chan struct{})
		e.metrics.changeMu.Unlock()
	}()
	if err := e.next.ExportSpans(ctx, spans); err != nil {
		e.metrics.failures.Add(1)
		e.metrics.batches.WithLabelValues("failure").Inc()
		e.metrics.failed.Add(float64(len(spans)))
		// Collector responses can contain sensitive details. The SDK error handler gets
		// only a sanitized failure, while the local counters retain the failure signal.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("trace exporter: export failed")
	}
	e.metrics.batches.WithLabelValues("success").Inc()
	e.metrics.exported.Add(float64(len(spans)))
	e.metrics.lastSuccess.SetToCurrentTime()
	return nil
}

func (e *monitoredTraceExporter) Shutdown(ctx context.Context) error {
	if err := e.next.Shutdown(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("trace exporter: shutdown failed")
	}
	return nil
}
