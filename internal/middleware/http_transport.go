package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/paveletto99/microservice-blueprint/pkg/observability"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type observedHTTPTransport struct {
	stack     *RequestStack
	next      http.RoundTripper
	operation string
}

// HTTPTransport observes through response body completion and propagates cloned trace context.
// operation must be a fixed upstream name, never a caller-controlled URL or identifier.
func (s *RequestStack) HTTPTransport(operation string, next http.RoundTripper) http.RoundTripper {
	return &observedHTTPTransport{stack: s, next: next, operation: operation}
}

func (t *observedHTTPTransport) RoundTrip(request *http.Request) (response *http.Response, err error) {
	ctx, observation := t.stack.telemetry.StartRequest(request.Context(), "http-client", t.operation, trace.SpanKindClient)
	transferred := false
	defer func() {
		p := recover()
		code, outcome := 0, "failure"
		if response != nil {
			code = response.StatusCode
			outcome = httpOutcome(code)
		}
		if err != nil {
			outcome = "failure"
		}
		if p != nil {
			outcome = "aborted"
		}
		if !transferred {
			observation.End(t.operation, strconv.Itoa(code), outcome)
		}
		if p != nil {
			panic(p)
		}
	}()
	clone := request.Clone(ctx)
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	observability.TracePropagator().Inject(ctx, propagation.HeaderCarrier(clone.Header))
	response, err = t.next.RoundTrip(clone)
	if err == nil && response != nil && response.Body != nil && response.Body != http.NoBody {
		body := &observedResponseBody{body: response.Body, observation: observation, operation: t.operation, status: response.StatusCode}
		remaining := int64(-1)
		if response.ContentLength > 0 {
			remaining = response.ContentLength
		}
		body.remaining.Store(remaining)
		response.Body = body
		transferred = true
	}
	return response, err
}

type observedResponseBody struct {
	body        io.ReadCloser
	observation *observability.RequestObservation
	operation   string
	status      int
	remaining   atomic.Int64
	complete    atomic.Bool
	end         sync.Once
}

func (b *observedResponseBody) finish(outcome string) {
	b.end.Do(func() { b.observation.End(b.operation, strconv.Itoa(b.status), outcome) })
}

func (b *observedResponseBody) Read(buffer []byte) (n int, err error) {
	defer func() {
		if p := recover(); p != nil {
			b.finish("aborted")
			panic(p)
		}
	}()
	n, err = b.body.Read(buffer)
	if err != nil && err != io.EOF {
		outcome := "failure"
		if errors.Is(err, context.Canceled) {
			outcome = "canceled"
		}
		b.finish(outcome)
		return n, err
	}
	finished := err == io.EOF
	if remaining := b.remaining.Load(); remaining >= 0 {
		finished = finished || b.remaining.Add(-int64(n)) <= 0
	}
	if finished {
		b.complete.Store(true)
		b.finish(httpOutcome(b.status))
	}
	return n, err
}

func (b *observedResponseBody) Close() (err error) {
	defer func() {
		if p := recover(); p != nil {
			b.finish("aborted")
			panic(p)
		}
	}()
	err = b.body.Close()
	outcome := "aborted"
	if err != nil {
		outcome = "failure"
	} else if b.complete.Load() {
		outcome = httpOutcome(b.status)
	}
	b.finish(outcome)
	return err
}
