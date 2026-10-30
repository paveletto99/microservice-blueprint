package logging

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestFromContextUsesInjectedLogger(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Sugar().With("request_id", "context-test")
	ctx := WithLogger(t.Context(), logger)
	if FromContext(ctx) != logger {
		t.Fatal("context logger was not preserved")
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	FromContext(child).Infow("context logger event", "operation", "test")
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("logged %d entries, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["request_id"] != "context-test" || fields["operation"] != "test" {
		t.Errorf("context logger lost fields: %v", fields)
	}
}

func TestFromContextFallsBackToDefaultLogger(t *testing.T) {
	t.Parallel()

	if FromContext(t.Context()) != DefaultLogger() {
		t.Fatal("missing context logger must use the shared default")
	}
}
