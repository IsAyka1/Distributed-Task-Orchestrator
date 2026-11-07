package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type ClaimRepository struct{ DB *sql.DB }
type claimTransaction struct{ *sql.Tx }

var _ queue.Repository = ClaimRepository{}

func (r ClaimRepository) Begin(ctx context.Context) (queue.Transaction, error) {
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	return claimTransaction{tx}, nil
}

func (t claimTransaction) LockNext(ctx context.Context, limit int) ([]queue.Candidate, error) {
	rows, err := t.QueryContext(ctx, claimWorkflowSQL, workflow.Running, task.Ready, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workflows []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		workflows = append(workflows, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Acquire every workflow lock before descending to wakeups and tasks.
	var candidates []queue.Candidate
	for _, id := range workflows {
		var wakeup string
		err = t.QueryRowContext(ctx, claimWakeupSQL, id).Scan(&wakeup)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		candidate := queue.Candidate{WorkflowID: id}
		err = t.QueryRowContext(ctx, claimTaskSQL, id, task.Ready).Scan(
			&candidate.ID, &candidate.Key, &candidate.Input.Value, &candidate.State.Status,
			&candidate.State.MaxAttemptCount, &candidate.State.AttemptCount, &candidate.AvailableAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	// Rechecked candidates may differ from the discovery snapshot.
	slices.SortFunc(candidates, func(a, b queue.Candidate) int {
		if order := a.AvailableAt.Compare(b.AvailableAt); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return candidates, nil
}

func (t claimTransaction) Now(ctx context.Context) (time.Time, error) {
	var at time.Time
	err := t.QueryRowContext(ctx, databaseNowSQL).Scan(&at)
	return at, err
}

func (t claimTransaction) InsertAttempt(ctx context.Context, start queue.AttemptStart) (queue.Attempt, error) {
	var attempt queue.Attempt
	err := t.QueryRowContext(ctx, claimInsertAttemptSQL, start.TaskID, start.Number, start.WorkerID, task.AttemptRunning, start.At).Scan(
		&attempt.ID, &attempt.TaskID, &attempt.WorkerID, &attempt.Number, &attempt.Status, &attempt.StartedAt, &attempt.HeartbeatAt)
	return attempt, err
}

func (t claimTransaction) UpdateTask(ctx context.Context, claimed queue.ClaimedTask) error {
	_, err := t.ExecContext(ctx, claimUpdateTaskSQL, claimed.TaskID, task.Running, claimed.Attempt.Number,
		claimed.Attempt.ID, claimed.Attempt.WorkerID, claimed.LeaseExpiresAt, claimed.Attempt.StartedAt)
	return err
}

func (t claimTransaction) Enqueue(ctx context.Context, id string, at time.Time) (queue.Wakeup, error) {
	var wakeup queue.Wakeup
	err := t.QueryRowContext(ctx, claimEnqueueSQL, id, at).Scan(&wakeup.WorkflowID, &wakeup.CreatedAt)
	return wakeup, err
}

func (t claimTransaction) IncrementVersion(ctx context.Context, id string) error {
	_, err := t.ExecContext(ctx, claimRevisionSQL, id)
	return err
}
