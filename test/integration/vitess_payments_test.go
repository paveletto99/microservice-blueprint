package integration_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/internal/payment"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
)

// TestVitessPayments requires an isolated development gateway and cleans up its unique fixture.
func TestVitessPayments(t *testing.T) {
	address := os.Getenv("VTGATE_TEST_ADDRESS")
	if address == "" {
		t.Skip("set VTGATE_TEST_ADDRESS for isolated development Vitess integration")
	}
	db, err := database.OpenVitess(t.Context(), database.VitessConfig{Address: address, Target: os.Getenv("VTGATE_TEST_TARGET"), MaxIdleConns: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if err := payment.MigratePayments(t.Context(), db.Pool); err != nil {
		t.Fatal(err)
	}
	if err := payment.MigratePayments(t.Context(), db.Pool); err != nil {
		t.Fatal(err)
	}
	if err := orders.Migrate(t.Context(), db.Pool); err != nil {
		t.Fatal(err)
	}
	store, err := payment.NewPaymentRepository(db.Pool)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(uuid.NewString()))
	input := payment.PaymentInput{OrderID: uuid.NewString(), AmountMinor: 125, Currency: "USD", IdempotencyDigest: hex.EncodeToString(digest[:])}
	t.Cleanup(func() {
		if _, err := db.Pool.ExecContext(context.Background(), "DELETE FROM simulated_payments WHERE idempotency_digest = :digest", sql.Named("digest", input.IdempotencyDigest)); err != nil {
			t.Error("payment fixture cleanup failed")
		}
	})
	results := make(chan payment.PaymentResult, 12)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.CreatePayment(t.Context(), input)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id string
	created := 0
	for result := range results {
		if id == "" {
			id = result.ID
		}
		if result.ID != id {
			t.Fatal("concurrent idempotency returned different IDs")
		}
		if !result.Replayed {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	reopened, err := payment.NewPaymentRepository(db.Pool)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := reopened.CreatePayment(t.Context(), input)
	if err != nil || replay.ID != id || !replay.Replayed {
		t.Fatal("persisted payment was not replayed")
	}
	input.AmountMinor++
	if _, err := store.CreatePayment(t.Context(), input); !errors.Is(err, payment.ErrPaymentConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}
