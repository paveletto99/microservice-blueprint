package main

import (
	"context"
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/demoruntime"
	"github.com/paveletto99/microservice-blueprint/internal/orders"
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
	if err := orders.Migrate(ctx, db.Pool); err != nil {
		return err
	}
	logging.FromContext(ctx).Info("orders schema migration verified")
	return nil
}
