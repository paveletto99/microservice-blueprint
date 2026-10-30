package setup

import (
	"testing"

	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/sethvargo/go-envconfig"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSetupWithLogsWithoutConfigurationValues(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.DebugLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())
	var config struct {
		Password string `env:"PASSWORD"`
	}
	env, err := SetupWith(ctx, &config, envconfig.MapLookuper(map[string]string{
		"PASSWORD": "test-only-value",
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { env.Close(ctx) })
	if config.Password != "test-only-value" {
		t.Fatal("configuration was not loaded")
	}
	entries := logs.FilterMessage("loaded service configuration").All()
	if len(entries) != 1 {
		t.Fatalf("logged %d configuration events, want 1", len(entries))
	}
	if len(entries[0].Context) != 0 {
		t.Fatal("configuration values must not appear in logs")
	}
}
