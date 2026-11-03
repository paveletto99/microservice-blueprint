// Package database owns the shared SQL pool handle used by services.
package database

import (
	"context"
	"database/sql"

	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// DB wraps the process-owned connection pool.
type DB struct {
	Pool *sql.DB
}

// Close releases database connections.
func (db *DB) Close(ctx context.Context) {
	logging.FromContext(ctx).Info("closing connection pool")
	db.Pool.Close()
}
