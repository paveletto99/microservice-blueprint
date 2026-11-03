package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingResponseBody struct{}

func (failingResponseBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingResponseBody) Close() error             { return nil }

func TestHTTPClientObservationIncludesResponseBodyFailure(t *testing.T) {
	stack, telemetry := testRequestStack(t, 1)
	transport := stack.HTTPTransport("service-b", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: -1, Body: failingResponseBody{}}, nil
	}))
	response, err := transport.RoundTrip(httptest.NewRequest("GET", "http://example.test/orders", nil))
	if err != nil {
		t.Fatal(err)
	}
	metrics := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `service_requests_in_flight{protocol="http-client",service="test"} 1`) {
		t.Fatal("client request ended at response headers")
	}
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error = %v", err)
	}
	_ = response.Body.Close()
	metrics = httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `service_requests_total{operation="service-b",outcome="failure",protocol="http-client",service="test",status="200"} 1`) || !strings.Contains(metrics.Body.String(), `service_requests_in_flight{protocol="http-client",service="test"} 0`) {
		t.Fatalf("missing body failure metric: %s", metrics.Body.String())
	}
}

func TestHTTPClientEarlyCloseCompletesOnlyOnce(t *testing.T) {
	stack, telemetry := testRequestStack(t, 1)
	transport := stack.HTTPTransport("service-b", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader("unread data"))}, nil
	}))
	response, err := transport.RoundTrip(httptest.NewRequest("GET", "http://example.test/orders", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	_ = response.Body.Close()
	metrics := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `outcome="aborted",protocol="http-client",service="test",status="200"} 1`) {
		t.Fatal("early close was not recorded exactly once")
	}
}
