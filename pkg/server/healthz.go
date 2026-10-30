package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paveletto99/microservice-blueprint/pkg/cache"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

func HandleHealthz(db *database.DB) http.Handler {
	cacher, _ := cache.New[bool](1 * time.Second)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		result, _ := cacher.WriteThruLookup("healthz", func() (bool, error) {
			conn, err := db.Pool.Conn(ctx)
			if err != nil {
				logging.FromContext(ctx).Errorw("failed to acquire database connection", "error", err)
				return false, nil
			}
			defer conn.Close()

			if err := conn.PingContext(ctx); err != nil {
				logging.FromContext(ctx).Errorw("failed to ping database", "error", err)
				return false, nil
			}

			return true, nil
		})

		if !result {
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status": "ok"}`)
	})
}
