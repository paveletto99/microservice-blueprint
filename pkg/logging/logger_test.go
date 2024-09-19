package logging

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"log/slog"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()

	logger := NewLogger("", true)
	if logger == nil {
		t.Fatal("expected logger to never be nil")
	}
}

func TestDefaultLogger(t *testing.T) {
	t.Parallel()

	logger1 := DefaultLogger()
	if logger1 == nil {
		t.Fatal("expected logger to never be nil")
	}

	logger2 := DefaultLogger()
	if logger2 == nil {
		t.Fatal("expected logger to never be nil")
	}

	// Intentionally comparing identities here
	if logger1 != logger2 {
		t.Errorf("expected %#v to be %#v", logger1, logger2)
	}
}

func TestContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger1 := FromContext(ctx)
	if logger1 == nil {
		t.Fatal("expected logger to never be nil")
	}

	ctx = WithLogger(ctx, logger1)

	logger2 := FromContext(ctx)
	if logger1 != logger2 {
		t.Errorf("expected %#v to be %#v", logger1, logger2)
	}
}

func TestLevelToSlogLevls(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  slog.Level
	}{
		{input: levelDebug, want: slog.LevelDebug},
		{input: levelInfo, want: slog.LevelInfo},
		{input: levelWarning, want: slog.LevelWarn},
		{input: levelError, want: slog.LevelError},
		{input: levelCritical, want: LevelCritical},
		{input: levelAlert, want: LevelAlert},
		{input: levelEmergency, want: LevelEmergency},
		{input: levelFatal, want: LevelFatal},
		{input: levelNotice, want: LevelNotice},
		{input: "unknown", want: slog.LevelWarn},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got := levelToSlogLevel(tc.input)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
