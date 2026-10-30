package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository implements order CRUD through the existing Vitess database/sql driver.
type Repository struct{ pool *sql.DB }

// NewRepository requires an injected pool; it never creates alternate database transports.
func NewRepository(pool *sql.DB) (*Repository, error) {
	if pool == nil {
		return nil, errors.New("orders: database pool is required")
	}
	return &Repository{pool: pool}, nil
}

func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}

// Health verifies connectivity and the required order columns without reading application data.
func (r *Repository) Health(ctx context.Context) error {
	ctx, cancel := queryContext(ctx)
	defer cancel()
	rows, err := r.pool.QueryContext(ctx, "SELECT id, item FROM orders LIMIT 0")
	if err != nil {
		return fmt.Errorf("orders: schema health check: %w", err)
	}
	defer rows.Close()
	return rows.Err()
}

// Create generates an opaque UUID and binds values by name; no client SQL is interpolated.
func (r *Repository) Create(ctx context.Context, item string) (Order, error) {
	item, err := NormalizeItem(item)
	if err != nil {
		return Order{}, err
	}
	ctx, cancel := queryContext(ctx)
	defer cancel()
	order := Order{ID: uuid.NewString(), Item: item}
	if _, err := r.pool.ExecContext(ctx, "INSERT INTO orders (id, item) VALUES (:id, :item)", sql.Named("id", order.ID), sql.Named("item", order.Item)); err != nil {
		return Order{}, fmt.Errorf("orders: create failed: %w", err)
	}
	return order, nil
}

// Get returns exactly one order or ErrNotFound.
func (r *Repository) Get(ctx context.Context, id string) (Order, error) {
	if err := ValidateID(id); err != nil {
		return Order{}, err
	}
	ctx, cancel := queryContext(ctx)
	defer cancel()
	var order Order
	err := r.pool.QueryRowContext(ctx, "SELECT id, item FROM orders WHERE id = :id", sql.Named("id", id)).Scan(&order.ID, &order.Item)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("orders: read failed: %w", err)
	}
	return order, nil
}

// List uses a bounded limit-plus-one query to expose a deterministic continuation cursor.
func (r *Repository) List(ctx context.Context, options ListOptions) (Page, error) {
	options, err := options.Validated()
	if err != nil {
		return Page{}, err
	}
	ctx, cancel := queryContext(ctx)
	defer cancel()
	rows, err := r.pool.QueryContext(ctx, "SELECT id, item FROM orders WHERE id > :after ORDER BY id LIMIT :limit", sql.Named("after", options.After), sql.Named("limit", options.Limit+1))
	if err != nil {
		return Page{}, fmt.Errorf("orders: list failed: %w", err)
	}
	defer rows.Close()
	page := Page{Orders: make([]Order, 0, options.Limit+1)}
	for rows.Next() {
		var order Order
		if err := rows.Scan(&order.ID, &order.Item); err != nil {
			return Page{}, fmt.Errorf("orders: list scan failed: %w", err)
		}
		page.Orders = append(page.Orders, order)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("orders: list iteration failed: %w", err)
	}
	if len(page.Orders) > options.Limit {
		page.Orders = page.Orders[:options.Limit]
		page.NextAfter = page.Orders[len(page.Orders)-1].ID
	}
	return page, nil
}

// Update locks the existing row in a transaction, so unchanged values remain successful updates.
func (r *Repository) Update(ctx context.Context, id, item string) (Order, error) {
	if err := ValidateID(id); err != nil {
		return Order{}, err
	}
	item, err := NormalizeItem(item)
	if err != nil {
		return Order{}, err
	}
	ctx, cancel := queryContext(ctx)
	defer cancel()
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, fmt.Errorf("orders: begin update: %w", err)
	}
	defer tx.Rollback() // Expected sql.ErrTxDone after commit; rollback also cleans up every earlier failure.
	var existing string
	if err := tx.QueryRowContext(ctx, "SELECT id FROM orders WHERE id = :id FOR UPDATE", sql.Named("id", id)).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	} else if err != nil {
		return Order{}, fmt.Errorf("orders: lock update row: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE orders SET item = :item WHERE id = :id", sql.Named("item", item), sql.Named("id", id)); err != nil {
		return Order{}, fmt.Errorf("orders: update failed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("orders: commit update: %w", err)
	}
	return Order{ID: id, Item: item}, nil
}

// Delete uses affected-row count to distinguish a missing order without a read/delete race.
func (r *Repository) Delete(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	ctx, cancel := queryContext(ctx)
	defer cancel()
	result, err := r.pool.ExecContext(ctx, "DELETE FROM orders WHERE id = :id", sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("orders: delete failed: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("orders: delete result failed: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
