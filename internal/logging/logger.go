// Package logging provides structured logging for the Uptimy Agent.
package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a *zap.SugaredLogger with the given level and format.
// Supported formats: "json" (default), "console".
func NewLogger(level, format string) (*zap.SugaredLogger, error) {
	var cfg zap.Config

	switch format {
	case "console":
		cfg = zap.NewDevelopmentConfig()
	default:
		cfg = zap.NewProductionConfig()
	}

	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	cfg.Level.SetLevel(lvl)

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("building logger: %w", err)
	}
	return logger.Sugar(), nil
}

// Nop returns a no-op SugaredLogger suitable for tests.
func Nop() *zap.SugaredLogger {
	return zap.NewNop().Sugar()
}
