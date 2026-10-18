package clock

import (
	"context"
	"sync"
	"time"
)

type timer struct {
	at     time.Time
	cancel context.CancelFunc
}
type Manual struct {
	mu      sync.Mutex
	now     time.Time
	timers  []timer
	Waiting chan struct{}
}

func New(at time.Time) *Manual { return &Manual{now: at, Waiting: make(chan struct{}, 1)} }
func (c *Manual) WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	c.mu.Lock()
	c.timers = append(c.timers, timer{c.now.Add(d), cancel})
	c.mu.Unlock()
	return ctx, cancel
}
func (c *Manual) Wait(ctx context.Context, d time.Duration) error {
	timeout, cancel := c.WithTimeout(ctx, d)
	defer cancel()
	select {
	case c.Waiting <- struct{}{}:
	default:
	}
	<-timeout.Done()
	return ctx.Err()
}
func (c *Manual) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, timer := range c.timers {
		if !timer.at.After(c.now) {
			timer.cancel()
		}
	}
}
