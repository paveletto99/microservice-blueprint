/*===========================================================================*\

\*===========================================================================*/

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paveletto99/go-pobo"
	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/internal/service"
	"github.com/paveletto99/microservice-blueprint/pkg/server"
	"github.com/urfave/cli/v2"
)

var cfg = &AppOptions{}

type AppOptions struct {
	verbose bool
}

func main() {
	// setup context
	_, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		done()
		if r := recover(); r != nil {
			slog.Error("application panic", "panic", r)
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
			ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

			defer func() {
				done()
				if r := recover(); r != nil {
					slog.Error("😱 application panic", "panic", r)
				}
			}()
			err := realMain(ctx)
			if err != nil {
				slog.Error("error running realMain", "error", err)
			}
			done()

			slog.Info("successful shutdown 🌂")
			return nil
		},
	}

	var err error

	// Load environment variables
	err = Environment()
	if err != nil {
		slog.Error("error loading environment", "error", err)
		os.Exit(99)
	}

	// Arbitrary (non-error) pre load
	Preloader()

	// Runtime
	err = app.Run(os.Args)
	if err != nil {
		slog.Error("error running app", "error", err)
		os.Exit(-1)
	}
}

// Preloader will run for ALL commands, and is used
// to initalize the runtime environments of the program.
func Preloader() {
	/* Flag parsing */
}

func realMain(ctx context.Context) error {

	var config service.Config

	// env, err := setup.Setup(ctx, &config)
	// if err != nil {
	// 	return fmt.Errorf("setup.Setup: %w", err)
	// }
	// defer env.Close(ctx)
	env := &serverenv.ServerEnv{}
	serviceServer, err := service.NewServer(&config, env)
	if err != nil {
		return fmt.Errorf("service.NewServer: %w", err)
	}

	srv, err := server.New(config.Port)
	if err != nil {
		return fmt.Errorf("server.New: %w", err)
	}
	slog.Info(fmt.Sprintf("listening on :%s", config.Port))

	return srv.ServeHTTPHandler(ctx, serviceServer.Run(ctx))
}
