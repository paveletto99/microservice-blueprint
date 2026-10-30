package api

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// NewServiceAHandler exposes local demo triggers, never caller-selected upstream hosts or credentials.
func NewServiceAHandler(oauth *auth.OAuthClient, resourceURL string) http.Handler {
	client := oauth.HTTPClient()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := oauth.AccessToken(r.Context()); err != nil {
			http.Error(w, "not ready", 503)
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})
	call := func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil || len(body) > 4096 {
			http.Error(w, "request body too large", 413)
			return
		}
		path := "/orders"
		if r.Method == http.MethodDelete {
			path += "/" + url.PathEscape(r.PathValue("id"))
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimSuffix(resourceURL, "/")+path, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "upstream request failed", 502)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			logging.FromContext(r.Context()).Warn("authenticated upstream call failed")
			http.Error(w, "upstream unavailable", 502)
			return
		}
		defer response.Body.Close() // Response is bounded and never contains access tokens.
		data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
		if err != nil || len(data) > 64*1024 {
			http.Error(w, "invalid upstream response", 502)
			return
		}
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		if _, err := w.Write(data); err != nil {
			logging.FromContext(r.Context()).Warn("upstream response could not be written")
		}
	}
	mux.HandleFunc("GET /call/orders", call)
	mux.HandleFunc("POST /call/orders", call)
	mux.HandleFunc("DELETE /call/orders/{id}", call)
	return mux
}
