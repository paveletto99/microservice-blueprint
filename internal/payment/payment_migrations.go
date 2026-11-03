package payment

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

//go:embed migrations/0001_payments.sql
var paymentSchema string

// MigratePayments applies the additive payment migration without changing order versions.
func MigratePayments(ctx context.Context, pool *sql.DB) error {
	if pool == nil {
		return errors.New("payment migration: database pool is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := pool.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS payment_schema_migrations (version BIGINT NOT NULL PRIMARY KEY, checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL)"); err != nil {
		return fmt.Errorf("payment migration: version table: %w", err)
	}
	var maximum sql.NullInt64
	if err := pool.QueryRowContext(ctx, "SELECT MAX(version) FROM payment_schema_migrations").Scan(&maximum); err != nil {
		return fmt.Errorf("payment migration: versions: %w", err)
	}
	if maximum.Valid && maximum.Int64 > 1 {
		return errors.New("payment migration: database schema is newer than this application")
	}
	digest := sha256.Sum256([]byte(paymentSchema))
	checksum := hex.EncodeToString(digest[:])
	var current string
	err := pool.QueryRowContext(ctx, "SELECT checksum FROM payment_schema_migrations WHERE version = 1").Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("payment migration: checksum: %w", err)
	}
	if err == nil && current != checksum {
		return errors.New("payment migration: checksum mismatch")
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := pool.ExecContext(ctx, paymentSchema); err != nil {
			return fmt.Errorf("payment migration: create payments: %w", err)
		}
	}
	if err := verifyPaymentSchema(ctx, pool); err != nil {
		return err
	}
	if _, err := pool.ExecContext(ctx, "INSERT IGNORE INTO payment_schema_migrations (version, checksum) VALUES (:version, :checksum)", sql.Named("version", 1), sql.Named("checksum", checksum)); err != nil {
		return fmt.Errorf("payment migration: record version: %w", err)
	}
	if err := pool.QueryRowContext(ctx, "SELECT checksum FROM payment_schema_migrations WHERE version = 1").Scan(&current); err != nil {
		return fmt.Errorf("payment migration: verify version: %w", err)
	}
	if current != checksum {
		return errors.New("payment migration: conflicting checksum")
	}
	return nil
}

func verifyPaymentSchema(ctx context.Context, pool *sql.DB) error {
	rows, err := pool.QueryContext(ctx, "SHOW FULL COLUMNS FROM simulated_payments")
	if err != nil {
		return fmt.Errorf("payment migration: inspect schema: %w", err)
	}
	defer rows.Close()
	type column struct{ kind, collation, key string }
	want := map[string]column{
		"id": {"char(36)", "ascii_bin", "UNI"}, "idempotency_digest": {"char(64)", "ascii_bin", "PRI"},
		"order_id": {"char(36)", "ascii_bin", ""}, "amount_minor": {"bigint", "", ""}, "currency": {"char(3)", "ascii_bin", ""},
	}
	seen := make(map[string]bool)
	for rows.Next() {
		var name, kind, nullable, key, extra, privileges, comment string
		var collation, defaultValue sql.NullString
		if err := rows.Scan(&name, &kind, &collation, &nullable, &key, &defaultValue, &extra, &privileges, &comment); err != nil {
			return fmt.Errorf("payment migration: inspect column: %w", err)
		}
		expected, ok := want[name]
		if !ok || seen[name] || expected != (column{kind, collation.String, key}) || nullable != "NO" || extra != "" {
			return errors.New("payment migration: incompatible payments table")
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("payment migration: schema iteration: %w", err)
	}
	if len(seen) != len(want) {
		return errors.New("payment migration: incomplete payments schema")
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("payment migration: close columns: %w", err)
	}
	indexes, err := pool.QueryContext(ctx, "SHOW INDEX FROM simulated_payments")
	if err != nil {
		return fmt.Errorf("payment migration: inspect indexes: %w", err)
	}
	defer indexes.Close()
	columns, err := indexes.Columns()
	if err != nil {
		return fmt.Errorf("payment migration: index columns: %w", err)
	}
	positions := make(map[string]int, len(columns))
	for i, name := range columns {
		positions[strings.ToLower(name)] = i
	}
	for _, name := range []string{"non_unique", "key_name", "seq_in_index", "column_name", "sub_part"} {
		if _, ok := positions[name]; !ok {
			return errors.New("payment migration: unsupported index metadata")
		}
	}
	expectedIndexes := map[string]string{"PRIMARY": "idempotency_digest", "payment_id": "id"}
	seenIndexes := make(map[string]bool)
	for indexes.Next() {
		values := make([]sql.RawBytes, len(columns))
		targets := make([]any, len(values))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := indexes.Scan(targets...); err != nil {
			return fmt.Errorf("payment migration: inspect index: %w", err)
		}
		key := string(values[positions["key_name"]])
		column, ok := expectedIndexes[key]
		if !ok || seenIndexes[key] || string(values[positions["non_unique"]]) != "0" || string(values[positions["seq_in_index"]]) != "1" || string(values[positions["column_name"]]) != column || values[positions["sub_part"]] != nil {
			return errors.New("payment migration: incompatible payments indexes")
		}
		seenIndexes[key] = true
	}
	if err := indexes.Err(); err != nil {
		return fmt.Errorf("payment migration: index iteration: %w", err)
	}
	if len(seenIndexes) != len(expectedIndexes) {
		return errors.New("payment migration: missing payments indexes")
	}
	return nil
}
