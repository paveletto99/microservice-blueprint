package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/paveletto99/microservice-blueprint/internal/api"
	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
)

// TestVitessOrders is explicitly enabled against a development VTGate; fixtures have unique IDs.
// Docker runs it in B's namespace so every real SQL operation crosses SPIFFE Envoy mTLS.
func TestVitessOrders(t *testing.T) {
	address := os.Getenv("VTGATE_TEST_ADDRESS")
	if address == "" {
		t.Skip("set VTGATE_TEST_ADDRESS for isolated development Vitess integration")
	}
	db, err := database.OpenVitess(t.Context(), database.VitessConfig{Address: address, Target: os.Getenv("VTGATE_TEST_TARGET"), MaxIdleConns: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if err := orders.Migrate(t.Context(), db.Pool); err != nil {
		t.Fatal(err)
	}
	store, err := orders.NewRepository(db.Pool)
	if err != nil {
		t.Fatal(err)
	}
	s := newStackWithStore(t, []string{"orders:reader", "orders:writer", "orders:admin", "orders:delete"}, []string{"orders:read", "orders:write", "orders:delete"}, "service-a", store)
	oauth, err := auth.NewOAuthClient(auth.OAuthClientConfig{TokenURL: s.issuer.URL + "/oauth/token", ResourceURL: s.serviceB.URL, ClientID: "service-a", ClientSecret: fixtureSecret, Scopes: []string{"orders:read", "orders:write", "orders:delete"}, HTTPClient: s.issuer.Client(), Now: s.clock.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oauth.Close)
	caller := httptest.NewTLSServer(api.NewServiceAHandler(oauth, s.serviceB.URL))
	t.Cleanup(caller.Close)
	client := caller.Client()
	id := createOrderHTTP(t, client, caller.URL+"/call/orders", "")
	t.Cleanup(func() {
		err := store.Delete(context.Background(), id)
		if err != nil && !errors.Is(err, orders.ErrNotFound) {
			t.Error("fixture cleanup failed")
		}
	})
	if err := orders.Migrate(t.Context(), db.Pool); err != nil {
		t.Fatal(err)
	}
	read, err := store.Get(t.Context(), id)
	if err != nil || read.Item != "fixture" {
		t.Fatal("migration did not preserve existing fixture")
	}
	for range 2 {
		if status := requestStatus(t, client, "PUT", caller.URL+"/call/orders/"+id, ""); status != 200 {
			t.Fatalf("same-value HTTP/Vitess update = %d", status)
		}
	}
	// Bind values, including SQL punctuation, must remain data rather than executable SQL.
	item := "bound'); DROP TABLE orders; --"
	input, err := json.Marshal(map[string]string{"item": item})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), "PUT", caller.URL+"/call/orders/"+id, strings.NewReader(string(input)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("bound data update failed")
	}
	read, err = store.Get(t.Context(), id)
	if err != nil || read.Item != item {
		t.Fatal("bound values did not round-trip")
	}
	if status := requestStatus(t, client, "GET", caller.URL+"/call/orders/"+id, ""); status != 200 {
		t.Fatal("single order read failed")
	}
	if status := requestStatus(t, client, "GET", caller.URL+"/call/orders?limit=1", ""); status != 200 {
		t.Fatal("bounded HTTP listing failed")
	}
	page, err := store.List(t.Context(), orders.ListOptions{Limit: 1})
	if err != nil || len(page.Orders) > 1 {
		t.Fatal("Vitess list limit failed")
	}
	if status := requestStatus(t, client, "DELETE", caller.URL+"/call/orders/"+id, ""); status != 204 {
		t.Fatal("order delete failed")
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if status := requestStatus(t, client, method, caller.URL+"/call/orders/"+id, ""); status != 404 {
			t.Fatalf("missing %s returned %d", method, status)
		}
	}
	if status := requestStatus(t, s.serviceB.Client(), "GET", s.serviceB.URL+"/orders", ""); status != 401 {
		t.Fatal("persistent route bypassed authentication")
	}
	if err := store.Health(t.Context()); err != nil {
		t.Fatal("schema health failed after CRUD")
	}
}
