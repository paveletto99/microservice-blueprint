package middleware

import (
	"net/http"

	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// Recovery recovers from panics and other fatal errors. It keeps the server and
// service running, returning 500 to the caller while also logging the error in
// a structured format.
func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := &responseState{ResponseWriter: w}
			defer func() {
				if p := recover(); p != nil {
					if p == http.ErrAbortHandler {
						panic(p)
					}
					// Panic values can contain credentials or payloads. Never serialize them.
					if state := requestStateFrom(r.Context()); state != nil {
						state.panicked = true
					}
					logging.FromContext(r.Context()).Error("http handler panic")
					if response.status != 0 {
						panic(http.ErrAbortHandler)
					}
					response.Header().Set("Content-Type", "text/plain; charset=utf-8")
					response.Header().Del("Location")
					http.Error(response, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(response, r)
		})
	}
}
