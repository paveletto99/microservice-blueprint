package middleware

import (
	"log/slog"
	"net/http"
)

// Recovery recovers from panics and other fatal errors. It keeps the server and
// service running, returning 500 to the caller while also logging the error in
// a structured format.
func Recovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					slog.Error("http handler panic", "panic", p)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}()
			next.ServeHTTP(w, r)
			return
		})
	}
}
