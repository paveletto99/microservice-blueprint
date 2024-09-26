package setup

import (
	"context"
	"fmt"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"

	"github.com/sethvargo/go-envconfig"
)

// Setup runs common initialization code for all servers. See SetupWith.
func Setup(ctx context.Context, config interface{}) (*serverenv.ServerEnv, error) {
	return SetupWith(ctx, config, envconfig.OsLookuper())
}

// SetupWith processes the given configuration using envconfig. It is
// responsible for establishing database connections, resolving secrets, and
// accessing app configs. The provided interface must implement the various
// interfaces.
func SetupWith(ctx context.Context, config interface{}, l envconfig.Lookuper) (*serverenv.ServerEnv, error) { //nolint:golint
	logger := logging.FromContext(ctx)

	// Build a list of mutators. This list will grow as we initialize more of the
	// configuration, such as the secret manager.
	var mutators []envconfig.Mutator

	// Build a list of options to pass to the server env.
	var serverEnvOpts []serverenv.Option

	c := &envconfig.Config{
		Target:   config,
		Lookuper: l,
		Mutators: mutators,
	}

	// Process first round of environment variables.
	if err := envconfig.ProcessWith(ctx, c); err != nil {
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}
	logger.Info("provided", "config", config)

	return serverenv.New(ctx, serverEnvOpts...), nil
}
