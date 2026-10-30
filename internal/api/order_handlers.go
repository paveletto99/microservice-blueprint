package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/paveletto99/microservice-blueprint/internal/auth"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

type orderHandlers struct{ store orders.Store }

func orderError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, orders.ErrNotFound):
		http.Error(w, "order not found", 404)
	case errors.Is(err, orders.ErrInvalidID), errors.Is(err, orders.ErrInvalidItem), errors.Is(err, orders.ErrInvalidPage):
		http.Error(w, "invalid order request", 400)
	default:
		// Driver errors may include bind values; do not log SQL errors or user-controlled items.
		logging.FromContext(r.Context()).Errorw("order storage operation failed", "operation", operation)
		http.Error(w, "order storage unavailable", 503)
	}
}

func decodeItem(w http.ResponseWriter, r *http.Request) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "application/json required", 415)
		return "", false
	}
	if r.Body == nil {
		http.Error(w, "order item required", 400)
		return "", false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			http.Error(w, "request body too large", 413)
		} else {
			http.Error(w, "invalid request body", 400)
		}
		return "", false
	}
	if !utf8.Valid(data) {
		http.Error(w, "invalid UTF-8 JSON", 400)
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	open, err := decoder.Token()
	if err != nil || open != json.Delim('{') {
		http.Error(w, "invalid order JSON", 400)
		return "", false
	}
	field, err := decoder.Token()
	if err != nil || field != "item" {
		http.Error(w, "only item is accepted", 400)
		return "", false
	}
	var item string
	if decoder.Decode(&item) != nil || decoder.More() {
		http.Error(w, "invalid or duplicate order fields", 400)
		return "", false
	}
	close, err := decoder.Token()
	if err != nil || close != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid order JSON", 400)
		return "", false
	}
	item, err = orders.NormalizeItem(item)
	if err != nil {
		http.Error(w, "invalid order item", 400)
		return "", false
	}
	return item, true
}

func orderJSON(w http.ResponseWriter, r *http.Request, status int, order orders.Order) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	jsonResponse(w, status, struct {
		Caller string       `json:"caller"`
		Order  orders.Order `json:"order"`
	}{claims.ClientID, order})
}

func (h orderHandlers) create(w http.ResponseWriter, r *http.Request) {
	item, ok := decodeItem(w, r)
	if !ok {
		return
	}
	order, err := h.store.Create(r.Context(), item)
	if err != nil {
		orderError(w, r, "create", err)
		return
	}
	w.Header().Set("Location", "/orders/"+order.ID)
	orderJSON(w, r, 201, order)
}

func (h orderHandlers) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := orders.ValidateID(id); err != nil {
		orderError(w, r, "get", err)
		return
	}
	order, err := h.store.Get(r.Context(), id)
	if err != nil {
		orderError(w, r, "get", err)
		return
	}
	orderJSON(w, r, 200, order)
}

func (h orderHandlers) list(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		orderError(w, r, "list", orders.ErrInvalidPage)
		return
	}
	for name, value := range values {
		if (name != "limit" && name != "after") || len(value) != 1 || value[0] == "" {
			orderError(w, r, "list", orders.ErrInvalidPage)
			return
		}
	}
	options := orders.ListOptions{After: values.Get("after")}
	if value := values.Get("limit"); value != "" {
		options.Limit, err = strconv.Atoi(value)
		if err != nil || options.Limit < 1 {
			orderError(w, r, "list", orders.ErrInvalidPage)
			return
		}
	}
	options, err = options.Validated()
	if err != nil {
		orderError(w, r, "list", err)
		return
	}
	page, err := h.store.List(r.Context(), options)
	if err != nil {
		orderError(w, r, "list", err)
		return
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	jsonResponse(w, 200, struct {
		Caller    string         `json:"caller"`
		Orders    []orders.Order `json:"orders"`
		NextAfter string         `json:"next_after,omitempty"`
	}{claims.ClientID, page.Orders, page.NextAfter})
}

func (h orderHandlers) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := orders.ValidateID(id); err != nil {
		orderError(w, r, "update", err)
		return
	}
	item, ok := decodeItem(w, r)
	if !ok {
		return
	}
	order, err := h.store.Update(r.Context(), id, item)
	if err != nil {
		orderError(w, r, "update", err)
		return
	}
	orderJSON(w, r, 200, order)
}

func (h orderHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := orders.ValidateID(id); err != nil {
		orderError(w, r, "delete", err)
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		orderError(w, r, "delete", err)
		return
	}
	w.WriteHeader(204)
}
