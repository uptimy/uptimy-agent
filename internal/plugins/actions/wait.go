package actions

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// WaitAction pauses execution for a configurable duration.
// It's useful as a cooldown or stabilization step in repair recipes.
type WaitAction struct {
	logger *zap.SugaredLogger
}

// NewWaitAction creates a WaitAction.
func NewWaitAction(logger *zap.SugaredLogger) *WaitAction {
	return &WaitAction{logger: logger}
}

// Name returns "wait".
func (a *WaitAction) Name() string { return "wait" }

// Execute pauses for the duration specified in params["duration"].
// Defaults to 10s if not specified or unparseable.
func (a *WaitAction) Execute(ctx context.Context, params map[string]string) error {
	d := 10 * time.Second
	if raw, ok := params["duration"]; ok {
		if parsed, err := time.ParseDuration(raw); err == nil {
			d = parsed
		} else {
			a.logger.Warnw("invalid wait duration, using default", "raw", raw, "default", d)
		}
	}

	a.logger.Infow("waiting", "duration", d)
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
