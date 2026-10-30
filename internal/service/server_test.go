/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
)

func TestQuicClient(t *testing.T) {
	t.Parallel()

	client, url := startHTTP3HealthzServer(t)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if response.ProtoMajor != 3 {
		t.Errorf("protocol = %s, want HTTP/3", response.Proto)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q, want application/json", got)
	}
	var health Healthz
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health.Status != "ok" {
		t.Errorf("health status = %q, want ok", health.Status)
	}
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	serviceMetrics := metrics.New(registry)
	serviceMetrics.ApiTotal.WithLabelValues("test", "http", "200").Inc()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := `api_total_requests{protocol="http",status="200",type="test"} 1`
	if !strings.Contains(string(body), want) {
		t.Errorf("metrics response does not contain %q", want)
	}
}

func startHTTP3HealthzServer(t *testing.T) (*http.Client, string) {
	t.Helper()

	// Reuse httptest's TLS setup without depending on repository certificates.
	tlsServer := httptest.NewTLSServer(nil)
	tlsConfig := tlsServer.TLS
	certificate := tlsServer.Certificate()
	tlsServer.Close()
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http3.Server{
		TLSConfig: tlsConfig,
		Handler:   addRoutes(http.NewServeMux()),
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(connection) }()
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(certificate)
	transport := &http3.Transport{TLSClientConfig: &tls.Config{RootCAs: rootCAs}}
	t.Cleanup(func() {
		if err := transport.Close(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-serveDone:
			if !errors.Is(err, http.ErrServerClosed) {
				t.Errorf("HTTP/3 server stopped with %v", err)
			}
		case <-ctx.Done():
			t.Error("HTTP/3 server did not stop")
		}
	})
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}, "https://" + connection.LocalAddr().String()
}
