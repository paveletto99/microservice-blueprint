// Package orders owns order persistence and validation, independent of HTTP authentication.
package orders

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrNotFound    = errors.New("orders: record not found")
	ErrInvalidID   = errors.New("orders: invalid order ID")
	ErrInvalidItem = errors.New("orders: invalid item")
	ErrInvalidPage = errors.New("orders: invalid pagination")
)

// Order is a globally scoped demo order; tenant ownership requires a separate model.
type Order struct {
	ID   string `json:"id"`
	Item string `json:"item"`
}

// ListOptions bounds keyset pagination and accepts a canonical UUID cursor.
type ListOptions struct {
	Limit int
	After string
}

// Page returns deterministic ID-ordered results and an optional next cursor.
type Page struct {
	Orders    []Order `json:"orders"`
	NextAfter string  `json:"next_after,omitempty"`
}

// Store is the persistence interface used by protected handlers and test fakes.
type Store interface {
	Create(context.Context, string) (Order, error)
	Get(context.Context, string) (Order, error)
	List(context.Context, ListOptions) (Page, error)
	Update(context.Context, string, string) (Order, error)
	Delete(context.Context, string) error
	Health(context.Context) error
}

// NormalizeItem enforces the VARCHAR(255) Unicode character bound and excludes control characters.
func NormalizeItem(item string) (string, error) {
	item = strings.TrimSpace(item)
	if item == "" || !utf8.ValidString(item) || utf8.RuneCountInString(item) > 255 {
		return "", ErrInvalidItem
	}
	for _, r := range item {
		if unicode.IsControl(r) {
			return "", ErrInvalidItem
		}
	}
	return item, nil
}

// ValidateID permits only canonical UUID strings, never arbitrary SQL identifiers.
func ValidateID(id string) error {
	value, err := uuid.Parse(id)
	if err != nil || value.String() != id {
		return ErrInvalidID
	}
	return nil
}

// Validated provides a default page size and rejects unbounded limits or malformed cursors.
func (o ListOptions) Validated() (ListOptions, error) {
	if o.Limit == 0 {
		o.Limit = 20
	}
	if o.Limit < 1 || o.Limit > 100 || (o.After != "" && ValidateID(o.After) != nil) {
		return o, ErrInvalidPage
	}
	return o, nil
}
