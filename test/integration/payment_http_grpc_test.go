package integration_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/client"
	"github.com/paveletto99/microservice-blueprint/internal/middleware"
	"github.com/paveletto99/microservice-blueprint/internal/payment"
	paymentv1 "github.com/paveletto99/microservice-blueprint/internal/pb/payment/v1"
	"github.com/paveletto99/microservice-blueprint/pkg/observability"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type testPaymentStore struct {
	mu     sync.Mutex
	inputs map[string]payment.PaymentInput
}

func (s *testPaymentStore) CreatePayment(_ context.Context, input payment.PaymentInput) (payment.PaymentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.inputs[input.IdempotencyDigest]
	if exists && previous != input {
		return payment.PaymentResult{}, payment.ErrPaymentConflict
	}
	s.inputs[input.IdempotencyDigest] = input
	return payment.PaymentResult{ID: "08095642-d644-499b-ae01-064e9be48b27", Replayed: exists}, nil
}

func TestPaymentHTTPToGRPC(t *testing.T) {
	s := newStack(t, []string{"orders:writer", "payments:writer"}, []string{"orders:write", "payments:write"}, "service-a")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	makeStack := func(service string) *middleware.RequestStack {
		t.Helper()
		telemetry, err := observability.NewRequestTelemetry(t.Context(), observability.RequestTelemetryConfig{ServiceName: service})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = telemetry.Close(context.Background()) })
		stack, err := middleware.NewRequestStack(middleware.DefaultRequestPolicy(), telemetry)
		if err != nil {
			t.Fatal(err)
		}
		return stack
	}
	paymentStack, bStack := makeStack("payment"), makeStack("service-b")
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(paymentStack.UnaryServerInterceptor()))
	store := &testPaymentStore{inputs: make(map[string]payment.PaymentInput)}
	paymentv1.RegisterPaymentServiceServer(grpcServer, payment.NewPaymentServer(store))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("payment.v1.PaymentService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	go grpcServer.Serve(listener)
	t.Cleanup(grpcServer.Stop)
	payments, err := client.NewPaymentClientWithInterceptor(listener.Addr().String(), bStack.UnaryClientInterceptor())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { payments.Close() })
	handler, err := api.BuildServiceBHandler(api.ServiceBHandlerConfig{Verifier: s.verifier, Orders: s.store, Payments: payments, AllowedClients: []string{"service-a"}, Requests: bStack})
	if err != nil {
		t.Fatal(err)
	}
	b := httptest.NewTLSServer(handler)
	t.Cleanup(b.Close)
	oauth, err := auth.NewOAuthClient(auth.OAuthClientConfig{TokenURL: s.issuer.URL + "/oauth/token", ResourceURL: b.URL, ClientID: "service-a", ClientSecret: fixtureSecret, Scopes: []string{"orders:write", "payments:write"}, HTTPClient: s.issuer.Client(), Now: s.clock.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oauth.Close)
	a := httptest.NewTLSServer(api.NewServiceAHandler(oauth, b.URL))
	t.Cleanup(a.Close)
	id := createOrderHTTP(t, a.Client(), a.URL+"/call/orders", "")
	endpoint := a.URL + "/call/orders/" + id + "/payments"
	request := func(body, key string) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		response, err := a.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]any
		if response.StatusCode == 200 || response.StatusCode == 201 {
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return response, result
	}
	for _, tc := range []struct {
		body, key string
		want      int
	}{
		{`{"amount_minor":123,"currency":"USD"}`, "", 400},
		{`{"amount_minor":1,"amount_minor":2,"currency":"USD"}`, "one", 400},
		{`{"amount_minor":0,"currency":"USD"}`, "one", 400},
		{`{"amount_minor":123,"currency":"USD"}`, "one", 201},
		{`{"amount_minor":123,"currency":"USD"}`, "one", 200},
		{`{"amount_minor":124,"currency":"USD"}`, "one", 409},
	} {
		resp, body := request(tc.body, tc.key)
		if resp.StatusCode != tc.want {
			t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
		}
		if tc.want == 201 || tc.want == 200 {
			if body["simulated"] != true || body["replayed"] != (tc.want == 200) || body["payment_id"] == "" {
				t.Fatalf("unexpected payment response: %v", body)
			}
		}
	}
	unauthorized, err := http.NewRequestWithContext(t.Context(), "POST", b.URL+"/orders/"+id+"/payments", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.Client().Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthorized status = %d", resp.StatusCode)
	}
	before := s.store.dataCalls.Load()
	token := s.token(t, "service-a", []string{"orders:write"})
	denied, err := http.NewRequestWithContext(t.Context(), "POST", b.URL+"/orders/"+id+"/payments", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	denied.Header.Set("Authorization", "Bearer "+token)
	resp, err = b.Client().Do(denied)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 || s.store.dataCalls.Load() != before {
		t.Fatal("payment authorization did not precede storage")
	}
	healthServer.SetServingStatus("payment.v1.PaymentService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	resp, err = b.Client().Get(b.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("payment not-ready status = %d", resp.StatusCode)
	}
}
