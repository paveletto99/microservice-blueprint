package api

import (
	"context"
	"errors"
	"net"
	"net/http"
)

func requestCancellationStatus(ctx context.Context, err error) (int, bool) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return http.StatusRequestTimeout, true
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, true
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return http.StatusGatewayTimeout, true
	}
	return 0, false
}

func writeRequestReadError(w http.ResponseWriter, r *http.Request, err error) {
	if code, expired := requestCancellationStatus(r.Context(), err); expired {
		http.Error(w, "request canceled or expired", code)
		return
	}
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid request body", http.StatusBadRequest)
}
