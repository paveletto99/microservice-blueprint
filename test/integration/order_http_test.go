package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/paveletto99/microservice-blueprint/internal/orders"
)

func createOrderHTTP(t *testing.T, client *http.Client, endpoint, token string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), "POST", endpoint, strings.NewReader(`{"item":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("create status = %d, want 201", response.StatusCode)
	}
	var body struct {
		Order orders.Order `json:"order"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if orders.ValidateID(body.Order.ID) != nil || body.Order.Item != "fixture" || response.Header.Get("Location") == "" {
		t.Fatal("invalid create response")
	}
	return body.Order.ID
}

func TestOrderInputValidationAndReadiness(t *testing.T) {
	s := newStack(t, []string{"orders:writer", "orders:reader", "orders:admin", "orders:delete"}, []string{"orders:write", "orders:read", "orders:delete"}, "service-a")
	token := s.token(t, "service-a", []string{"orders:write", "orders:read", "orders:delete"})
	for _, tc := range []struct {
		body, contentType string
		status            int
	}{
		{"", "application/json", 400}, {`{}`, "application/json", 400},
		{`{"item":" "}`, "application/json", 400}, {`{"item":"x","role":"admin"}`, "application/json", 400},
		{`{"item":"x","item":"y"}`, "application/json", 400}, {`{"item":1}`, "application/json", 400},
		{`{"item":"x"} {}`, "application/json", 400}, {strings.Repeat("x", 4097), "application/json", 413},
		{`{"item":"x"}`, "text/plain", 415},
		{"{\"item\":\"" + string([]byte{0xff}) + "\"}", "application/json", 400},
	} {
		req, err := http.NewRequestWithContext(t.Context(), "POST", s.serviceB.URL+"/orders", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", tc.contentType)
		response, err := s.serviceB.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("invalid input status = %d, want %d", response.StatusCode, tc.status)
		}
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=x", "?after=bad", "?limit=1&limit=2", "?unknown=1"} {
		if status := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+"/orders"+query, token); status != 400 {
			t.Fatalf("invalid page returned %d", status)
		}
	}
	if s.store.dataCalls.Load() != 0 {
		t.Fatal("invalid input accessed storage")
	}
	id := createOrderHTTP(t, s.serviceB.Client(), s.serviceB.URL+"/orders", token)
	if status := requestStatus(t, s.serviceB.Client(), "PUT", s.serviceB.URL+"/orders/"+id, token); status != 200 {
		t.Fatal("same-value update rejected")
	}
	if status := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+"/orders?limit=1", token); status != 200 {
		t.Fatal("bounded listing failed")
	}
	if status := requestStatus(t, s.serviceB.Client(), "DELETE", s.serviceB.URL+"/orders/"+id, token); status != 204 {
		t.Fatal("delete failed")
	}
	if status := requestStatus(t, s.serviceB.Client(), "PUT", s.serviceB.URL+"/orders/"+id, token); status != 404 {
		t.Fatal("missing update did not return 404")
	}
	if status := requestStatus(t, s.serviceB.Client(), "DELETE", s.serviceB.URL+"/orders/"+id, token); status != 404 {
		t.Fatal("missing delete did not return 404")
	}
	s.store.mu.Lock()
	s.store.unavailable = true
	s.store.mu.Unlock()
	if status := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+"/readyz", ""); status != 503 {
		t.Fatal("readiness ignored database failure")
	}
	if status := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+"/healthz", ""); status != 200 {
		t.Fatal("database failure changed process liveness")
	}
}
