package payment

import (
	"context"
	"errors"
	"strings"
	"testing"

	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type paymentStoreFake struct {
	calls int
	err   error
}

func (s *paymentStoreFake) CreatePayment(_ context.Context, input PaymentInput) (PaymentResult, error) {
	s.calls++
	return PaymentResult{ID: "08095642-d644-499b-ae01-064e9be48b27", Replayed: true}, s.err
}

func TestCreatePaymentValidationAndErrors(t *testing.T) {
	valid := func() *paymentv1.CreatePaymentRequest {
		return &paymentv1.CreatePaymentRequest{OrderId: "95b3af78-1772-40ce-bdec-0599ec539ed4", AmountMinor: 123, Currency: "USD", IdempotencyDigest: strings.Repeat("a", 64)}
	}
	for _, tc := range []struct {
		name string
		edit func(*paymentv1.CreatePaymentRequest)
	}{
		{"order", func(r *paymentv1.CreatePaymentRequest) { r.OrderId = "bad" }},
		{"amount", func(r *paymentv1.CreatePaymentRequest) { r.AmountMinor = 0 }},
		{"currency", func(r *paymentv1.CreatePaymentRequest) { r.Currency = "usd" }},
		{"digest", func(r *paymentv1.CreatePaymentRequest) { r.IdempotencyDigest = "bad" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &paymentStoreFake{}
			server := NewPaymentServer(store)
			req := valid()
			tc.edit(req)
			_, err := server.CreatePayment(t.Context(), req)
			if status.Code(err) != codes.InvalidArgument || store.calls != 0 {
				t.Fatalf("status = %v, store calls = %d", status.Code(err), store.calls)
			}
		})
	}
	store := &paymentStoreFake{}
	server := NewPaymentServer(store)
	response, err := server.CreatePayment(t.Context(), valid())
	if err != nil || response.PaymentId == "" || !response.Replayed {
		t.Fatalf("response = %v, error = %v", response, err)
	}
	store.err = ErrPaymentConflict
	if _, err := server.CreatePayment(t.Context(), valid()); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("conflict status = %v", status.Code(err))
	}
	store.err = errors.New("database detail")
	if _, err := server.CreatePayment(t.Context(), valid()); status.Code(err) != codes.Unavailable || strings.Contains(err.Error(), "database detail") {
		t.Fatalf("storage error = %v", err)
	}
}
