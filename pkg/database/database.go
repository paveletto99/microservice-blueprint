package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotFound indicates that the requested record was not found in the database.
	ErrNotFound = errors.New("record not found")

	// ErrKeyConflict indicates that there was a key conflict inserting a row.
	ErrKeyConflict = errors.New("key conflict")
)

func NullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (db *DB) InTx(ctx context.Context, txOpt *sql.TxOptions, f func(tx *sql.Tx) error) error {

	tx, err := db.Pool.BeginTx(ctx, txOpt)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}

	if err := f(tx); err != nil {
		if err1 := tx.Rollback(); err1 != nil {
			return fmt.Errorf("rolling back transaction: %v (original error: %w)", err1, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
