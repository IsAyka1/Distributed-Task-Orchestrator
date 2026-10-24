package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	testclock "github.com/IsAyka1/Distributed-Task-Orchestrator/tests/support/clock"
)

// Pause after all real inserts, then let database/sql finish its automatic rollback.
type cancellingStartRepository struct {
	workflows.Repository
	db    *sql.DB
	clock *testclock.Manual
	mode  string
}

type cancellingStartTransaction struct {
	workflows.Transaction
	repository cancellingStartRepository
	ctx        context.Context
}

func (r cancellingStartRepository) Begin(ctx context.Context) (workflows.Transaction, error) {
	tx, err := r.Repository.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return cancellingStartTransaction{Transaction: tx, repository: r, ctx: ctx}, nil
}

func (t cancellingStartTransaction) Commit() error {
	if t.repository.mode == "deadline" {
		t.repository.clock.Advance(time.Second)
	} else {
		fmt.Println("start ready to commit")
	}
	<-t.ctx.Done()
	// With one pool connection, acquiring it proves rollback has released the transaction.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := t.repository.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("rollback barrier: %w", err)
	}
	if err := conn.Close(); err != nil {
		return err
	}
	err = t.Transaction.Commit()
	if !errors.Is(err, sql.ErrTxDone) {
		return errors.New("expected automatically rolled back transaction")
	}
	fmt.Println("start automatically rolled back")
	return err
}
