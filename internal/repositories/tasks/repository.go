package tasks

import (
	"context"
	"database/sql"
	"errors"
	"time"

	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository struct{ DB *sql.DB }
type transaction struct{ *sql.Tx }

var _ service.Repository = Repository{}

func (r Repository) Begin(ctx context.Context) (service.Transaction, error) {
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	return transaction{tx}, nil
}

func (t transaction) LockAttempt(ctx context.Context, request service.CompletionRequest) (service.Snapshot, error) {
	var current service.Snapshot
	err := t.QueryRowContext(ctx, lockWorkflowSQL, request.TaskID).Scan(&current.WorkflowID, &current.WorkflowStatus)
	if err != nil {
		return current, lookupError(err)
	}
	var wakeup string
	err = t.QueryRowContext(ctx, lockWakeupSQL, current.WorkflowID).Scan(&wakeup)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return current, err
	}
	err = t.QueryRowContext(ctx, lockTaskSQL, request.TaskID, current.WorkflowID).Scan(
		&current.TaskID, &current.State.Status, &current.State.MaxAttemptCount, &current.State.AttemptCount,
		&current.ActiveAttemptID, &current.LeaseOwner, &current.LeaseExpiresAt)
	if err != nil {
		return current, lookupError(err)
	}
	// Let PostgreSQL normalize UUIDs before comparing the requested and active tokens.
	err = t.QueryRowContext(ctx, lockAttemptSQL, current.TaskID, request.AttemptID).Scan(
		&current.AttemptID, &current.AttemptWorkerID, &current.State.LastAttempt.Number, &current.State.LastAttempt.Status)
	return current, lookupError(err)
}

func (t transaction) Now(ctx context.Context) (time.Time, error) {
	var at time.Time
	err := t.QueryRowContext(ctx, nowSQL).Scan(&at)
	return at, err
}

func (t transaction) CloseAttempt(ctx context.Context, c service.Completion) error {
	_, err := t.ExecContext(ctx, closeAttemptSQL, c.AttemptID, c.State.LastAttempt.Status, c.At, c.ErrorType, c.ErrorMessage)
	return err
}

func (t transaction) CompleteTask(ctx context.Context, c service.Completion) error {
	_, err := t.ExecContext(ctx, completeTaskSQL, c.TaskID, c.State.Status, []byte(c.Output.Value), c.At)
	return requestError(err)
}

func (t transaction) Enqueue(ctx context.Context, id string, at time.Time) (service.Wakeup, error) {
	var wakeup service.Wakeup
	err := t.QueryRowContext(ctx, enqueueSQL, id, at).Scan(&wakeup.WorkflowID, &wakeup.CreatedAt)
	return wakeup, err
}

func (t transaction) IncrementVersion(ctx context.Context, id string) error {
	_, err := t.ExecContext(ctx, incrementVersionSQL, id)
	return err
}

func lookupError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrStaleAttempt
	}
	return requestError(err)
}

func requestError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22021" || pgErr.Code == "22P05" || pgErr.Code == "22003") {
		return service.ErrInvalidRequest
	}
	return err
}
