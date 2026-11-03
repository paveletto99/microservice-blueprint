package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// NewServiceAHandler exposes local demo triggers, never caller-selected upstream hosts or credentials.
// Deprecated: production startup uses BuildServiceAHandler with an explicit request stack.
func NewServiceAHandler(oauth *auth.OAuthClient, resourceURL string) http.Handler {
	requests, err := defaultServiceRequests("service-a")
	if err == nil {
		var handler http.Handler
		handler, err = BuildServiceAHandler(ServiceAHandlerConfig{OAuth: oauth, ResourceURL: resourceURL, Requests: requests})
		if err == nil {
			return handler
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid service configuration", http.StatusInternalServerError)
	})
}

// ServiceAHandlerConfig keeps the local-only trigger distinct from protected resource APIs.
type ServiceAHandlerConfig struct {
	OAuth       *auth.OAuthClient
	ResourceURL string
	Requests    *middleware.RequestStack
}

// BuildServiceAHandler applies the shared operational stack without inventing caller identity.
func BuildServiceAHandler(config ServiceAHandlerConfig) (http.Handler, error) {
	oauth, resourceURL, requests := config.OAuth, config.ResourceURL, config.Requests
	if oauth == nil || requests == nil || resourceURL == "" {
		return nil, errors.New("API: OAuth client, resource URL and request stack are required")
	}
	if err := requests.RegisterDependency("oauth", func(ctx context.Context) error { _, err := oauth.AccessToken(ctx); return err }); err != nil {
		return nil, err
	}
	client := oauth.HTTPClient()
	client.Transport = requests.HTTPTransport("service-b", client.Transport)
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", requests.OperationalHTTP("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, map[string]string{"status": "ok"}) })))
	mux.Handle("GET /readyz", requests.OperationalHTTP("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if _, err := oauth.AccessToken(ctx); err != nil {
			http.Error(w, "not ready", 503)
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ready"})
	})))
	mux.Handle("GET /metrics", requests.OperationalHTTP("GET /metrics", requests.MetricsHandler()))
	call := func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil {
			writeRequestReadError(w, r, err)
			return
		}
		if len(body) > 4096 {
			http.Error(w, "request body too large", 413)
			return
		}
		path := "/orders"
		if id := r.PathValue("id"); id != "" {
			path += "/" + url.PathEscape(id)
		}
		if strings.HasSuffix(r.URL.Path, "/payments") {
			path += "/payments"
		}
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimSuffix(resourceURL, "/")+path, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "upstream request failed", 502)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/payments") {
			req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
			for _, key := range r.Header.Values("Idempotency-Key") {
				req.Header.Add("Idempotency-Key", key)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			if code, canceled := requestCancellationStatus(r.Context(), err); canceled {
				http.Error(w, "upstream request canceled or expired", code)
				return
			}
			http.Error(w, "upstream unavailable", 502)
			return
		}
		defer response.Body.Close() // Response is bounded and never contains access tokens.
		data, err := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
		if err != nil || len(data) > 256*1024 {
			http.Error(w, "invalid upstream response", 502)
			return
		}
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		if location := response.Header.Get("Location"); strings.HasPrefix(location, "/orders/") {
			w.Header().Set("Location", "/call"+location)
		}
		w.WriteHeader(response.StatusCode)
		if _, err := w.Write(data); err != nil {
			logging.FromContext(r.Context()).Warn("upstream response could not be written")
		}
	}
	for _, pattern := range []string{"GET /call/orders", "GET /call/orders/{id}", "POST /call/orders", "PUT /call/orders/{id}", "DELETE /call/orders/{id}", "POST /call/orders/{id}/payments"} {
		mux.Handle(pattern, requests.BusinessHTTP(pattern, http.HandlerFunc(call)))
	}
	return requests.HTTP(mux), nil
}
