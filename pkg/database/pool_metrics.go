package database

import (
	"database/sql"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

// DatabasePoolMetrics observes SQL pool saturation without recording queries or connection strings.
type DatabasePoolMetrics struct {
	stats                                                                                  func() sql.DBStats
	connections, capacity, waits, waitDuration, idleClosed, idleTimeClosed, lifetimeClosed *prometheus.Desc
}

// NewDatabasePoolMetrics uses the existing pool; it never opens a second database connection.
func NewDatabasePoolMetrics(pool *sql.DB) (*DatabasePoolMetrics, error) {
	if pool == nil {
		return nil, errors.New("database metrics: pool is required")
	}
	return newDatabasePoolMetrics(pool.Stats), nil
}

func newDatabasePoolMetrics(stats func() sql.DBStats) *DatabasePoolMetrics {
	return &DatabasePoolMetrics{stats: stats,
		connections:    prometheus.NewDesc("database_pool_connections", "SQL connections by state.", []string{"state"}, nil),
		capacity:       prometheus.NewDesc("database_pool_connection_capacity", "Maximum open SQL connections.", nil, nil),
		waits:          prometheus.NewDesc("database_pool_wait_total", "SQL connection waits.", nil, nil),
		waitDuration:   prometheus.NewDesc("database_pool_wait_duration_seconds_total", "Cumulative SQL connection wait duration.", nil, nil),
		idleClosed:     prometheus.NewDesc("database_pool_idle_closed_total", "Connections closed by the idle limit.", nil, nil),
		idleTimeClosed: prometheus.NewDesc("database_pool_idle_time_closed_total", "Connections closed by idle lifetime.", nil, nil),
		lifetimeClosed: prometheus.NewDesc("database_pool_lifetime_closed_total", "Connections closed by maximum lifetime.", nil, nil),
	}
}

// Describe provides fixed metric descriptors with no caller-controlled labels.
func (m *DatabasePoolMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, descriptor := range []*prometheus.Desc{m.connections, m.capacity, m.waits, m.waitDuration, m.idleClosed, m.idleTimeClosed, m.lifetimeClosed} {
		ch <- descriptor
	}
}

// Collect reads one consistent pool snapshot; collection performs no database I/O.
func (m *DatabasePoolMetrics) Collect(ch chan<- prometheus.Metric) {
	s := m.stats()
	ch <- prometheus.MustNewConstMetric(m.connections, prometheus.GaugeValue, float64(s.OpenConnections), "open")
	ch <- prometheus.MustNewConstMetric(m.connections, prometheus.GaugeValue, float64(s.InUse), "in_use")
	ch <- prometheus.MustNewConstMetric(m.connections, prometheus.GaugeValue, float64(s.Idle), "idle")
	ch <- prometheus.MustNewConstMetric(m.capacity, prometheus.GaugeValue, float64(s.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(m.waits, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(m.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds())
	ch <- prometheus.MustNewConstMetric(m.idleClosed, prometheus.CounterValue, float64(s.MaxIdleClosed))
	ch <- prometheus.MustNewConstMetric(m.idleTimeClosed, prometheus.CounterValue, float64(s.MaxIdleTimeClosed))
	ch <- prometheus.MustNewConstMetric(m.lifetimeClosed, prometheus.CounterValue, float64(s.MaxLifetimeClosed))
}
