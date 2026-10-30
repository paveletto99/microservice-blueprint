package integration_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
)

// Test-local store isolates authorization/HTTP tests from a real database.
// Real repository behavior is exercised by the Vitess/Envoy CRUD probe.
type fakeOrderStore struct {
	mu          sync.Mutex
	items       map[string]orders.Order
	dataCalls   atomic.Int64
	unavailable bool
}

func newOrderStore() *fakeOrderStore { return &fakeOrderStore{items: make(map[string]orders.Order)} }
func (f *fakeOrderStore) Health(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unavailable {
		return errors.New("fixture unavailable")
	}
	return nil
}
func (f *fakeOrderStore) Create(_ context.Context, item string) (orders.Order, error) {
	f.dataCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	order := orders.Order{ID: uuid.NewString(), Item: item}
	f.items[order.ID] = order
	return order, nil
}
func (f *fakeOrderStore) Get(_ context.Context, id string) (orders.Order, error) {
	f.dataCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	order, ok := f.items[id]
	if !ok {
		return orders.Order{}, orders.ErrNotFound
	}
	return order, nil
}
func (f *fakeOrderStore) List(_ context.Context, options orders.ListOptions) (orders.Page, error) {
	f.dataCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	page := orders.Page{Orders: []orders.Order{}}
	for id, order := range f.items {
		if id > options.After {
			page.Orders = append(page.Orders, order)
		}
	}
	sort.Slice(page.Orders, func(i, j int) bool { return page.Orders[i].ID < page.Orders[j].ID })
	if len(page.Orders) > options.Limit {
		page.Orders = page.Orders[:options.Limit]
		page.NextAfter = page.Orders[len(page.Orders)-1].ID
	}
	return page, nil
}
func (f *fakeOrderStore) Update(_ context.Context, id, item string) (orders.Order, error) {
	f.dataCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[id]; !ok {
		return orders.Order{}, orders.ErrNotFound
	}
	order := orders.Order{ID: id, Item: item}
	f.items[id] = order
	return order, nil
}
func (f *fakeOrderStore) Delete(_ context.Context, id string) error {
	f.dataCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[id]; !ok {
		return orders.ErrNotFound
	}
	delete(f.items, id)
	return nil
}
