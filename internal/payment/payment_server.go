package payment

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/paveletto99/microservice-blueprint/internal/orders"
	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PaymentInput identifies one simulated payment; AmountMinor is in currency minor units.
type PaymentInput struct {
	OrderID           string
	AmountMinor       int64
	Currency          string
	IdempotencyDigest string
}

// PaymentResult identifies a persisted simulated payment and whether it was replayed.
type PaymentResult struct {
	ID       string
	Replayed bool
}

// ErrPaymentConflict indicates an idempotency key was reused with different data.
var ErrPaymentConflict = errors.New("payment: idempotency conflict")

// PaymentStore persists simulated payments atomically by idempotency digest.
type PaymentStore interface {
	CreatePayment(context.Context, PaymentInput) (PaymentResult, error)
}

// PaymentServer implements the versioned simulated payment RPC.
type PaymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	store PaymentStore
}

// NewPaymentServer injects the store used for simulated payment records.
func NewPaymentServer(store PaymentStore) *PaymentServer { return &PaymentServer{store: store} }

// CreatePayment validates and persists a simulated payment without charging money.
func (s *PaymentServer) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	if req == nil || orders.ValidateID(req.OrderId) != nil || req.AmountMinor <= 0 || req.AmountMinor > 1_000_000_000_000 || !validCurrency(req.Currency) || !validDigest(req.IdempotencyDigest) {
		return nil, status.Error(codes.InvalidArgument, "invalid payment request")
	}
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "payment store unavailable")
	}
	result, err := s.store.CreatePayment(ctx, PaymentInput{OrderID: req.OrderId, AmountMinor: req.AmountMinor, Currency: req.Currency, IdempotencyDigest: req.IdempotencyDigest})
	if errors.Is(err, ErrPaymentConflict) {
		return nil, status.Error(codes.AlreadyExists, "payment idempotency conflict")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, status.FromContextError(err).Err()
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "payment store unavailable")
	}
	return &paymentv1.CreatePaymentResponse{PaymentId: result.ID, Replayed: result.Replayed}, nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, c := range currency {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func validDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
