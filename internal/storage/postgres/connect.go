package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL: use a PostgreSQL connection string")
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("connect to PostgreSQL: check DATABASE_URL, access and DATABASE_TIMEOUT")
	}
	return db, nil
}

func Now(ctx context.Context, db *sql.DB) (time.Time, error) {
	var now time.Time
	err := db.QueryRowContext(ctx, databaseNowSQL).Scan(&now)
	return now, err
}
