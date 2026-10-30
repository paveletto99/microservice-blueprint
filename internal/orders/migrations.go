package orders

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

//go:embed migrations/0001_orders.sql
var orderSchema string

// Migrate applies the additive order migration and verifies its checksum/schema.
// MySQL DDL implicitly commits: idempotent DDL plus schema checks recover interrupted runs.
// This never drops tables, rewrites existing records or resets database volumes.
func Migrate(ctx context.Context, pool *sql.DB) error {
	if pool == nil {
		return errors.New("orders migration: database pool is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := pool.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS service_auth_schema_migrations (version BIGINT NOT NULL PRIMARY KEY, checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL)")
	if err != nil {
		return fmt.Errorf("orders migration: create version table: %w", err)
	}
	var maximum sql.NullInt64
	if err := pool.QueryRowContext(ctx, "SELECT MAX(version) FROM service_auth_schema_migrations").Scan(&maximum); err != nil {
		return fmt.Errorf("orders migration: read versions: %w", err)
	}
	if maximum.Valid && maximum.Int64 > 1 {
		return errors.New("orders migration: database schema is newer than this application")
	}
	digest := sha256.Sum256([]byte(orderSchema))
	checksum := hex.EncodeToString(digest[:])
	var current string
	err = pool.QueryRowContext(ctx, "SELECT checksum FROM service_auth_schema_migrations WHERE version = 1").Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("orders migration: read checksum: %w", err)
	}
	if err == nil && current != checksum {
		return errors.New("orders migration: checksum mismatch; manual review required")
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := pool.ExecContext(ctx, orderSchema); err != nil {
			return fmt.Errorf("orders migration: create orders: %w", err)
		}
	}
	if err := verifyOrderSchema(ctx, pool); err != nil {
		return err
	}
	if _, err := pool.ExecContext(ctx, "INSERT IGNORE INTO service_auth_schema_migrations (version, checksum) VALUES (:version, :checksum)", sql.Named("version", 1), sql.Named("checksum", checksum)); err != nil {
		return fmt.Errorf("orders migration: record version: %w", err)
	}
	if err := pool.QueryRowContext(ctx, "SELECT checksum FROM service_auth_schema_migrations WHERE version = 1").Scan(&current); err != nil {
		return fmt.Errorf("orders migration: verify version: %w", err)
	}
	if current != checksum {
		return errors.New("orders migration: conflicting migration checksum")
	}
	return nil
}

func verifyOrderSchema(ctx context.Context, pool *sql.DB) error {
	rows, err := pool.QueryContext(ctx, "SHOW FULL COLUMNS FROM orders")
	if err != nil {
		return fmt.Errorf("orders migration: inspect schema: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]bool, 2)
	for rows.Next() {
		var name, kind, nullable, key, extra, privileges, comment string
		var collation, defaultValue sql.NullString
		if err := rows.Scan(&name, &kind, &collation, &nullable, &key, &defaultValue, &extra, &privileges, &comment); err != nil {
			return fmt.Errorf("orders migration: inspect column: %w", err)
		}
		valid := name == "id" && kind == "char(36)" && collation.String == "ascii_bin" && key == "PRI" || name == "item" && kind == "varchar(255)" && collation.String == "utf8mb4_bin" && key == ""
		if !valid || nullable != "NO" || extra != "" || seen[name] {
			return errors.New("orders migration: incompatible existing orders table; no data changed")
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orders migration: schema iteration: %w", err)
	}
	if len(seen) != 2 {
		return errors.New("orders migration: incomplete orders schema")
	}
	return nil
}
