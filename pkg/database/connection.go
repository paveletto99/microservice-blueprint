// Package database is a facade over the data storage layer.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	vit "vitess.io/vitess/go/vt/vitessdriver"
)

type DB struct {
	Pool *sql.DB
}

// NewFromEnv sets up the database connections using the configuration in the
// process's environment variables. This should be called just once per server
// instance.
func NewFromEnv(ctx context.Context, cfg *Config) (*DB, error) {
	db, err := vit.Open("localhost:15991", "@primary")

	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	//ping vitess
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping vitess: %w", err)
	}
	// Set the connection pool settings.

	return &DB{Pool: db}, nil
}

// Close releases database connections.
func (db *DB) Close(ctx context.Context) {
	slog.Info("Closing connection pool.")
	db.Pool.Close()
}

// dbDSN builds a connection string suitable for the pgx Postgres driver, using
// the values of vars.
func dbDSN(cfg *Config) string {
	vals := dbValues(cfg)
	p := make([]string, 0, len(vals))
	for k, v := range vals {
		p = append(p, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(p, " ")
}

func setIfNotEmpty(m map[string]string, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func setIfPositive(m map[string]string, key string, val int) {
	if val > 0 {
		m[key] = fmt.Sprintf("%d", val)
	}
}

func setIfPositiveDuration(m map[string]string, key string, d time.Duration) {
	if d > 0 {
		m[key] = d.String()
	}
}

func dbValues(cfg *Config) map[string]string {
	p := map[string]string{}
	setIfNotEmpty(p, "dbname", cfg.Name)
	setIfNotEmpty(p, "user", cfg.User)
	setIfNotEmpty(p, "host", cfg.Host)
	setIfNotEmpty(p, "port", cfg.Port)
	setIfNotEmpty(p, "sslmode", cfg.SSLMode)
	setIfPositive(p, "connect_timeout", cfg.ConnectionTimeout)
	setIfNotEmpty(p, "password", cfg.Password)
	setIfNotEmpty(p, "sslcert", cfg.SSLCertPath)
	setIfNotEmpty(p, "sslkey", cfg.SSLKeyPath)
	setIfNotEmpty(p, "sslrootcert", cfg.SSLRootCertPath)
	setIfNotEmpty(p, "pool_min_conns", cfg.PoolMinConnections)
	setIfNotEmpty(p, "pool_max_conns", cfg.PoolMaxConnections)
	setIfPositiveDuration(p, "pool_max_conn_lifetime", cfg.PoolMaxConnLife)
	setIfPositiveDuration(p, "pool_max_conn_idle_time", cfg.PoolMaxConnIdle)
	setIfPositiveDuration(p, "pool_health_check_period", cfg.PoolHealthCheck)
	return p
}
