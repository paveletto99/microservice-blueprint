/*===========================================================================*\

\*===========================================================================*/

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paveletto99/go-pobo"
	"github.com/paveletto99/microservice-blueprint/internal/backup"
	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
	"github.com/urfave/cli/v2"
)

var cfg = &AppOptions{}

type AppOptions struct {
	verbose bool
}

func main() {
	// setup context
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	logger := logging.DefaultLogger()
	defer logger.Sync()
	ctx = logging.WithLogger(ctx, logger)
	defer func() {
		done()
		if r := recover(); r != nil {
			logger.Errorw("application panic", "panic", r)
		}
	}()

	/* Change version to -V */
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "The version of the program.",
	}
	app := &cli.App{
		Name:     pobo.Name,
		Version:  pobo.Version,
		Compiled: time.Now(),
		Authors: []*cli.Author{
			{
				Name:  pobo.AuthorName,
				Email: pobo.AuthorEmail,
			},
		},
		Copyright: pobo.Copyright,
		HelpName:  pobo.Copyright,
		Usage:     "A go program.",
		UsageText: `service <options> <flags>
A longer sentence, about how exactly to use this program`,
		Commands: []*cli.Command{
			{},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Destination: &cfg.verbose,
			},
		},
		EnableBashCompletion: true,
		HideHelp:             false,
		HideVersion:          false,
		Action: func(c *cli.Context) error {
			ctx, done := signal.NotifyContext(c.Context, syscall.SIGINT, syscall.SIGTERM)

			defer func() {
				done()
				if r := recover(); r != nil {
					logger.Errorw("application panic", "panic", r)
				}
			}()
			err := realMain(ctx)
			if err != nil {
				logging.FromContext(ctx).Errorw("error running realMain", "error", err)
			}
			done()

			logging.FromContext(ctx).Info("successful shutdown")
			return nil
		},
	}

	var err error

	// Load environment variables
	err = Environment()
	if err != nil {
		logger.Errorw("error loading environment", "error", err)
		os.Exit(99)
	}

	// Arbitrary (non-error) pre load
	Preloader()

	// Runtime
	err = app.RunContext(ctx, os.Args)
	if err != nil {
		logger.Errorw("error running app", "error", err)
		os.Exit(-1)
	}
}

// Preloader will run for ALL commands, and is used
// to initalize the runtime environments of the program.
func Preloader() {
	/* Flag parsing */
}

func realMain(ctx context.Context) error {

	var config backup.Config
	// Setup the environment and configuration
	env, err := setup.Setup(ctx, &config)
	if err != nil {
		return fmt.Errorf("setup.Setup: %w", err)
	}
	defer env.Close(ctx)

	backupServer, err := backup.NewServer(&config, env)
	if err != nil {
		return fmt.Errorf("backup.NewServer: %w", err)
	}

	srv, err := server.New(config.Port)
	if err != nil {
		return fmt.Errorf("server.New: %w", err)
	}
	logging.FromContext(ctx).Infow("listening", "port", config.Port)

	return srv.ServeHTTPHandler(ctx, backupServer.Run(ctx))
}
