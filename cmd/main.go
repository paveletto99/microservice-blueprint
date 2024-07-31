/*===========================================================================*\

\*===========================================================================*/

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paveletto99/go-pobo"
	"github.com/paveletto99/go-pobo/internal/service"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var cfg = &AppOptions{}

type AppOptions struct {
	verbose bool
}

func main() {
	// setup logger
	var log = logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetOutput(os.Stdout)
	// setup context
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		done()
		if r := recover(); r != nil {
			log.Fatal("application panic", "panic", r)
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
			poboObject, _ := service.NewServer(log, &service.Config{Port: "6660"})
			err := poboObject.Run(ctx)
			done()
			if err != nil {
				log.Fatal(err)
			}
			log.Info("successful shutdown 🌂")
			return err
		},
	}

	var err error

	// Load environment variables
	err = Environment()
	if err != nil {
		logrus.Error(err)
		os.Exit(99)
	}

	// Arbitrary (non-error) pre load
	Preloader()

	// Runtime
	err = app.Run(os.Args)
	if err != nil {
		logrus.Error(err)
		os.Exit(-1)
	}
}

// Preloader will run for ALL commands, and is used
// to initalize the runtime environments of the program.
func Preloader() {
	/* Flag parsing */
	if cfg.verbose {
		logrus.SetLevel(logrus.InfoLevel)
	} else {
		logrus.SetLevel(logrus.WarnLevel)
	}
}
