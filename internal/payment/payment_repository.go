package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PaymentRepository stores simulated payments in Vitess; the pool remains caller-owned.
type PaymentRepository struct{ pool *sql.DB }

// NewPaymentRepository requires a configured Vitess SQL pool.
func NewPaymentRepository(pool *sql.DB) (*PaymentRepository, error) {
	if pool == nil {
		return nil, errors.New("payment: database pool is required")
	}
	return &PaymentRepository{pool: pool}, nil
}

// Health checks that the payment schema can be queried.
func (r *PaymentRepository) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := r.pool.QueryContext(ctx, "SELECT id, idempotency_digest, order_id, amount_minor, currency FROM simulated_payments LIMIT 0")
	if err != nil {
		return fmt.Errorf("payment: schema health check: %w", err)
	}
	defer rows.Close()
	return rows.Err()
}

// CreatePayment atomically inserts or replays a payment by its unique idempotency digest.
func (r *PaymentRepository) CreatePayment(ctx context.Context, input PaymentInput) (PaymentResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := uuid.NewString()
	_, err := r.pool.ExecContext(ctx, `INSERT INTO simulated_payments (id, idempotency_digest, order_id, amount_minor, currency)
VALUES (:id, :digest, :order_id, :amount_minor, :currency) ON DUPLICATE KEY UPDATE id = id`,
		sql.Named("id", id), sql.Named("digest", input.IdempotencyDigest), sql.Named("order_id", input.OrderID), sql.Named("amount_minor", input.AmountMinor), sql.Named("currency", input.Currency))
	if err != nil {
		return PaymentResult{}, fmt.Errorf("payment: insert: %w", err)
	}
	var storedID, orderID, currency string
	var amount int64
	err = r.pool.QueryRowContext(ctx, "SELECT id, order_id, amount_minor, currency FROM simulated_payments WHERE idempotency_digest = :digest", sql.Named("digest", input.IdempotencyDigest)).Scan(&storedID, &orderID, &amount, &currency)
	if err != nil {
		return PaymentResult{}, fmt.Errorf("payment: read: %w", err)
	}
	if orderID != input.OrderID || amount != input.AmountMinor || currency != input.Currency {
		return PaymentResult{}, ErrPaymentConflict
	}
	return PaymentResult{ID: storedID, Replayed: storedID != id}, nil
}
