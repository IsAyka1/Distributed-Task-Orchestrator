package postgres

import (
	"context"
	"database/sql"
	"testing"
)

type lockerFake struct {
	called  bool
	bounded bool
}

func (l *lockerFake) SessionLock(context.Context, *sql.Conn) error { return nil }
func (l *lockerFake) SessionUnlock(ctx context.Context, _ *sql.Conn) error {
	l.called = true
	_, l.bounded = ctx.Deadline()
	return ctx.Err()
}
func TestMigrationCleanupIsBounded(t *testing.T) {
	fake := &lockerFake{}
	err := (boundedLocker{fake}).SessionUnlock(context.Background(), nil)
	if err != nil || !fake.called || !fake.bounded {
		t.Fatal("cleanup must delegate with a deadline")
	}
}
func TestMigrationCleanupHonorsCancellation(t *testing.T) {
	fake := &lockerFake{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (boundedLocker{fake}).SessionUnlock(ctx, nil); err != context.Canceled {
		t.Fatal("cleanup lost cancellation")
	}
}
