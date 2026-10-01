package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

type closeProbe struct{ closed bool }

func (p *closeProbe) Connect(context.Context) (driver.Conn, error) { return p, nil }
func (p *closeProbe) Driver() driver.Driver                        { return nil }
func (p *closeProbe) Prepare(string) (driver.Stmt, error)          { return nil, errors.New("not used") }
func (p *closeProbe) Begin() (driver.Tx, error)                    { return nil, errors.New("not used") }
func (p *closeProbe) Close() error                                 { p.closed = true; return nil }

func TestRunClosesConnectionBeforeReturningError(t *testing.T) {
	probe := &closeProbe{}
	open := func(ctx context.Context, _ string) (*sql.DB, error) {
		db := sql.OpenDB(probe)
		if err := db.PingContext(ctx); err != nil {
			t.Fatal(err)
		}
		return db, nil
	}
	if err := run([]string{"-now", "invalid"}, open); err == nil {
		t.Fatal("expected invalid clock error")
	}
	if !probe.closed {
		t.Fatal("connection leaked on command error")
	}
}
