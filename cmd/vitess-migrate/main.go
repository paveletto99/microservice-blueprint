package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
	"github.com/paveletto99/microservice-blueprint/internal/payment"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

func main() { demoruntime.Main(realMain) }

func realMain(ctx context.Context) error {
	cfg, err := database.VitessConfigFromEnv()
	if err != nil {
		return err
	}
	db, err := database.OpenVitess(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch os.Getenv("MIGRATION_TARGET") {
	case "", "orders":
		if err := orders.Migrate(ctx, db.Pool); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("orders schema migration verified")
	case "payment":
		if err := payment.MigratePayments(ctx, db.Pool); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("payment schema migration verified")
	default:
		return errors.New("migration: invalid target")
	}
	return nil
}
