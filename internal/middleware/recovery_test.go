package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestContext returns a context with test values pre-populated.
func testContext(tb testing.TB) context.Context {
	ctx := context.Background()
	return ctx
}

func TestRecoveryUsesContextLogger(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar().With("request_id", "recovery-test")
	ctx := logging.WithLogger(t.Context(), logger)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	middleware.Recovery()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("oops")
	})).ServeHTTP(response, req)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("logged %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Level != zapcore.ErrorLevel || entry.Message != "http handler panic" {
		t.Fatalf("unexpected panic log: %v", entry.Entry)
	}
	fields := entry.ContextMap()
	if _, exposed := fields["panic"]; exposed || fields["request_id"] != "recovery-test" {
		t.Errorf("panic log exposed raw values or lost request context: %v", fields)
	}
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)

	m := middleware.Recovery()

	cases := []struct {
		name    string
		handler http.Handler
		code    int
	}{
		{
			name: "default",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}),
			code: http.StatusOK,
		},
		{
			name: "panic",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("oops")
			}),
			code: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
			if err != nil {
				t.Fatal(err)
			}

			w := httptest.NewRecorder()

			m(tc.handler).ServeHTTP(w, r)

			if got, want := w.Code, tc.code; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}
		})
	}
}
