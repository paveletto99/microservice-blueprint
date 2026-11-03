package database

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestDatabasePoolMetricsUsesSecondsAndFixedStates(t *testing.T) {
	collector := newDatabasePoolMetrics(func() sql.DBStats {
		return sql.DBStats{MaxOpenConnections: 10, OpenConnections: 4, InUse: 3, Idle: 1, WaitCount: 2, WaitDuration: 1500 * time.Millisecond}
	})
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	response := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	for _, metric := range []string{`database_pool_connections{state="in_use"} 3`, `database_pool_connection_capacity 10`, `database_pool_wait_duration_seconds_total 1.5`} {
		if !strings.Contains(response.Body.String(), metric) {
			t.Fatalf("missing metric %s", metric)
		}
	}
}
