package service

import "context"

type eventExecutionMode string

const (
	eventExecutionModeLive       eventExecutionMode = "live"
	eventExecutionModeHistorical eventExecutionMode = "historical"
)

type eventExecutionModeKey struct{}

func WithHistoricalReplay(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, eventExecutionModeKey{}, eventExecutionModeHistorical)
}

func historicalReplay(ctx context.Context) bool {
	mode, _ := ctx.Value(eventExecutionModeKey{}).(eventExecutionMode)
	return mode == eventExecutionModeHistorical
}

func contextWithDefaultReplayMode(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(eventExecutionModeKey{}).(eventExecutionMode); ok {
		return ctx
	}
	return context.WithValue(ctx, eventExecutionModeKey{}, eventExecutionModeLive)
}
