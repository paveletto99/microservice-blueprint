package logging

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestNewLoggerFromEnvDefaultsToInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_MODE", "")

	logger := NewLoggerFromEnv()

	if !logger.Desugar().Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected default logger to enable info level")
	}
}

func TestNewLoggerFromEnvHonorsExplicitLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "ERROR")
	t.Setenv("LOG_MODE", "")

	logger := NewLoggerFromEnv()

	if logger.Desugar().Core().Enabled(zapcore.InfoLevel) {
		t.Fatal("expected explicit ERROR level to disable info logs")
	}
	if !logger.Desugar().Core().Enabled(zapcore.ErrorLevel) {
		t.Fatal("expected explicit ERROR level to enable error logs")
	}
}
