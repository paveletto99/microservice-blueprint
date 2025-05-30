package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paveletto99/microservice-blueprint/internal/middleware"
)

// TestContext returns a context with test values pre-populated.
func testContext(tb testing.TB) context.Context {
	ctx := context.Background()
	return ctx
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
