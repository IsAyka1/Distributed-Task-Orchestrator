package worker

import (
	"context"
	"time"
)

type Clock interface {
	WithTimeout(context.Context, time.Duration) (context.Context, context.CancelFunc)
	Wait(context.Context, time.Duration) error
}
type SystemClock struct{}

func (SystemClock) WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
func (SystemClock) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
