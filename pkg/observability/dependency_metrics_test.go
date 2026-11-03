package observability

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestDependencyMetricReportsReadinessNotTransportSuccess(t *testing.T) {
	metric, err := DependencyReadinessMetric("database", func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("unbounded dependency probe")
		}
		return errors.New("sensitive database error")
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(metric)
	response := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(response.Body.String(), `service_dependency_ready{dependency="database"} 0`) || strings.Contains(response.Body.String(), "sensitive") {
		t.Fatal("unready dependency was not reported safely")
	}
}

func TestDependencyProbePanicDoesNotCrashCollector(t *testing.T) {
	metric, err := DependencyReadinessMetric("payment", func(context.Context) error { panic("sensitive dependency panic") })
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(metric)
	response := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(response.Body.String(), `service_dependency_ready{dependency="payment"} 0`) || strings.Contains(response.Body.String(), "sensitive") {
		t.Fatal("dependency panic was not contained")
	}
}
