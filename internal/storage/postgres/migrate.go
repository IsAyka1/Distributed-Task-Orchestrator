package postgres

import (
	"context"
	"database/sql"
	"io/fs"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func Migrate(ctx context.Context, db *sql.DB, source fs.FS) error {
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 60), lock.WithUnlockTimeout(1, 1))
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source,
		goose.WithDisableGlobalRegistry(true), goose.WithLogger(goose.NopLogger()),
		goose.WithSessionLocker(boundedLocker{locker}))
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

type boundedLocker struct{ lock.SessionLocker }

func (l boundedLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	// Goose detaches cleanup from the command context; bound that extra work too.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return l.SessionLocker.SessionUnlock(ctx, conn)
}
