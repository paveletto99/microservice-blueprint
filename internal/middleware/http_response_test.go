package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// plainResponseWriter deliberately has no optional streaming interfaces.
type plainResponseWriter struct{ recorder *httptest.ResponseRecorder }

func (w *plainResponseWriter) Header() http.Header            { return w.recorder.Header() }
func (w *plainResponseWriter) WriteHeader(code int)           { w.recorder.WriteHeader(code) }
func (w *plainResponseWriter) Write(body []byte) (int, error) { return w.recorder.Write(body) }

func TestUnsupportedFlushDoesNotCommitHeaders(t *testing.T) {
	writer := &responseState{ResponseWriter: &plainResponseWriter{recorder: httptest.NewRecorder()}}
	if err := writer.FlushError(); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("flush error = %v", err)
	}
	if writer.status != 0 || writer.writeFailed {
		t.Fatal("unsupported flush committed or failed the response")
	}
	writer.WriteHeader(503)
	if writer.status != 503 {
		t.Fatal("response could not be written after unsupported flush")
	}
}

func TestInvalidHTTPStatusCanRecoverBeforeCommit(t *testing.T) {
	response := httptest.NewRecorder()
	Recovery()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(1000) })).ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 500 {
		t.Fatalf("invalid response status recovery = %d", response.Code)
	}
}
