package middleware

import (
	"errors"
	"net/http"
)

// responseState tracks final headers while retaining ResponseController access to the writer.
type responseState struct {
	http.ResponseWriter
	status      int
	writeFailed bool
}

func (w *responseState) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseState) WriteHeader(code int) {
	if code < 100 || code > 999 {
		panic("invalid HTTP response status")
	}
	if w.status != 0 {
		return
	}
	if code >= 200 || code == http.StatusSwitchingProtocols {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseState) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		w.writeFailed = true
	}
	return n, err
}

func (w *responseState) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		if w.status == 0 {
			w.status = http.StatusOK
		}
		if err != nil {
			w.writeFailed = true
		}
	}
	return err
}
