// dbprobe exercises the existing Vitess gRPC driver through the local Envoy listener.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/pkg/database"

	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"vitess.io/vitess/go/vt/vitessdriver"
)

func main() {
	logger := logging.DefaultLogger()
	if err := query(); err != nil {
		logger.Errorw("Vitess gRPC mTLS probe failed", "error", err)
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "--crud" {
		if err := crud(); err != nil {
			logger.Errorw("Vitess CRUD probe failed", "error", err)
			os.Exit(1)
		}
		logger.Info("Vitess CRUD, same-value update and idempotent migration through SPIFFE Envoy passed")
	}
	logger.Info("Vitess gRPC query through SPIFFE Envoy passed")
}

func crud() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.OpenVitess(ctx, database.VitessConfig{Address: "127.0.0.1:15991", Target: "commerce@primary", MaxIdleConns: 1})
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if err := orders.Migrate(ctx, db.Pool); err != nil {
		return err
	}
	repository, err := orders.NewRepository(db.Pool)
	if err != nil {
		return err
	}
	if err := repository.Health(ctx); err != nil {
		return err
	}
	item := "fixture'); DROP TABLE orders; --"
	order, err := repository.Create(ctx, item)
	if err != nil {
		return err
	}
	defer repository.Delete(context.Background(), order.ID) // Clean only this uniquely generated fixture, including failure paths.
	read, err := repository.Get(ctx, order.ID)
	if err != nil || read.Item != item {
		return fmt.Errorf("bound order data did not round-trip")
	}
	if err := orders.Migrate(ctx, db.Pool); err != nil {
		return err
	}
	if _, err := repository.Get(ctx, order.ID); err != nil {
		return fmt.Errorf("migration changed existing data: %w", err)
	}
	for range 2 {
		updated, err := repository.Update(ctx, order.ID, "updated-é")
		if err != nil || updated.Item != "updated-é" {
			return fmt.Errorf("same-value order update failed: %w", err)
		}
	}
	page, err := repository.List(ctx, orders.ListOptions{Limit: 1})
	if err != nil || len(page.Orders) > 1 {
		return fmt.Errorf("bounded order listing failed: %w", err)
	}
	if page.NextAfter != "" && orders.ValidateID(page.NextAfter) != nil {
		return fmt.Errorf("invalid order continuation cursor")
	}
	if err := repository.Delete(ctx, order.ID); err != nil {
		return err
	}
	if _, err := repository.Get(ctx, order.ID); !errors.Is(err, orders.ErrNotFound) {
		return fmt.Errorf("deleted order was not missing")
	}
	if err := repository.Delete(ctx, order.ID); !errors.Is(err, orders.ErrNotFound) {
		return fmt.Errorf("missing delete was not detected")
	}
	if _, err := repository.Update(ctx, uuid.NewString(), "missing"); !errors.Is(err, orders.ErrNotFound) {
		return fmt.Errorf("missing update was not detected")
	}
	return nil
}

func query() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := vitessdriver.Open("127.0.0.1:15991", "commerce@primary")
	if err != nil {
		return fmt.Errorf("open VTGate probe: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var value int64
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&value); err != nil {
		return fmt.Errorf("VTGate probe query: %w", err)
	}
	if value != 1 {
		return fmt.Errorf("VTGate probe returned unexpected value")
	}
	return nil
}
