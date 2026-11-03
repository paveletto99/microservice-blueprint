package integration_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/paveletto99/microservice-blueprint/internal/client"
	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestSPIFFEPaymentRPC exercises the real payment RPC and Vitess over isolated Envoy proxies.
func TestSPIFFEPaymentRPC(t *testing.T) {
	address := os.Getenv("PAYMENT_TEST_ADDRESS")
	dbAddress := os.Getenv("VTGATE_TEST_ADDRESS")
	if address == "" || dbAddress == "" {
		t.Skip("set payment and isolated Vitess test addresses")
	}
	db, err := database.OpenVitess(t.Context(), database.VitessConfig{Address: dbAddress, Target: os.Getenv("VTGATE_TEST_TARGET"), MaxIdleConns: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	c, err := client.NewPaymentClient(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if err := c.Health(t.Context()); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(uuid.NewString()))
	request := &paymentv1.CreatePaymentRequest{OrderId: uuid.NewString(), AmountMinor: 123, Currency: "USD", IdempotencyDigest: hex.EncodeToString(digest[:])}
	t.Cleanup(func() {
		if _, err := db.Pool.ExecContext(context.Background(), "DELETE FROM simulated_payments WHERE idempotency_digest = :digest", sql.Named("digest", request.IdempotencyDigest)); err != nil {
			t.Error("payment RPC fixture cleanup failed")
		}
	})
	responses := make(chan *paymentv1.CreatePaymentResponse, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := c.CreatePayment(t.Context(), request)
			responses <- response
			errs <- err
		}()
	}
	wg.Wait()
	close(responses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	created := 0
	for response := range responses {
		if id == "" {
			id = response.PaymentId
		}
		if response.PaymentId != id || id == "" {
			t.Fatal("RPC replay returned different IDs")
		}
		if !response.Replayed {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	conflicting := &paymentv1.CreatePaymentRequest{OrderId: request.OrderId, AmountMinor: 124, Currency: request.Currency, IdempotencyDigest: request.IdempotencyDigest}
	if _, err := c.CreatePayment(t.Context(), conflicting); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("RPC conflict = %v", err)
	}
}
