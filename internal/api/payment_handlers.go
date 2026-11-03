package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PaymentCaller is the Service B dependency for payment RPCs and readiness.
type PaymentCaller interface {
	CreatePayment(context.Context, *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error)
	Health(context.Context) error
}

type paymentHandlers struct {
	store    orders.Store
	payments PaymentCaller
}

func (h paymentHandlers) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validPaymentKey(keys[0]) {
		http.Error(w, "invalid idempotency key", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if orders.ValidateID(id) != nil {
		http.Error(w, "invalid order ID", http.StatusBadRequest)
		return
	}
	amount, currency, ok := decodePayment(w, r)
	if !ok {
		return
	}
	if _, err := h.store.Get(r.Context(), id); err != nil {
		orderError(w, r, "payment order lookup", err)
		return
	}
	hash := sha256.New()
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(claims.ClientID)))
	hash.Write(size[:])
	hash.Write([]byte(claims.ClientID))
	hash.Write([]byte(keys[0]))
	response, err := h.payments.CreatePayment(r.Context(), &paymentv1.CreatePaymentRequest{OrderId: id, AmountMinor: amount, Currency: currency, IdempotencyDigest: hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			http.Error(w, "invalid payment request", http.StatusBadRequest)
		case codes.AlreadyExists:
			http.Error(w, "payment idempotency conflict", http.StatusConflict)
		case codes.DeadlineExceeded:
			http.Error(w, "payment service timeout", http.StatusGatewayTimeout)
		case codes.Canceled:
			http.Error(w, "payment request canceled", http.StatusRequestTimeout)
		case codes.ResourceExhausted:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "payment service capacity exhausted", http.StatusServiceUnavailable)
		default:
			http.Error(w, "payment service unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	if response == nil || orders.ValidateID(response.PaymentId) != nil {
		http.Error(w, "invalid payment service response", http.StatusBadGateway)
		return
	}
	code := http.StatusCreated
	if response.Replayed {
		code = http.StatusOK
	}
	jsonResponse(w, code, struct {
		PaymentID string `json:"payment_id"`
		Replayed  bool   `json:"replayed"`
		Simulated bool   `json:"simulated"`
	}{response.PaymentId, response.Replayed, true})
}

func validPaymentKey(key string) bool {
	if len(key) < 1 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e || c == ',' {
			return false
		}
	}
	return true
}

func decodePayment(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return 0, "", false
	}
	if r.Body == nil {
		http.Error(w, "payment body required", http.StatusBadRequest)
		return 0, "", false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeRequestReadError(w, r, err)
		return 0, "", false
	}
	invalid := func() (int64, string, bool) {
		http.Error(w, "invalid payment JSON", http.StatusBadRequest)
		return 0, "", false
	}
	if !utf8.Valid(data) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return invalid()
	}
	seen := map[string]bool{}
	var amount int64
	var currency string
	for decoder.More() {
		field, err := decoder.Token()
		name, ok := field.(string)
		if err != nil || !ok || seen[name] {
			return invalid()
		}
		seen[name] = true
		switch name {
		case "amount_minor":
			err = decoder.Decode(&amount)
		case "currency":
			err = decoder.Decode(&currency)
		default:
			return invalid()
		}
		if err != nil {
			return invalid()
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(seen) != 2 || amount <= 0 || amount > 1_000_000_000_000 || len(currency) != 3 {
		return invalid()
	}
	for _, c := range currency {
		if c < 'A' || c > 'Z' {
			return invalid()
		}
	}
	return amount, currency, true
}
