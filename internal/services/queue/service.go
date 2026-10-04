package queue

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var (
	ErrNoTask         = errors.New("no_task_available")
	ErrInvalidRequest = errors.New("invalid_request")
)

type ClaimRequest struct {
	WorkerID      string
	LeaseDuration time.Duration
}

type Candidate struct {
	ID, WorkflowID, Key string
	Input               workflow.Payload
	State               task.State
	AvailableAt         time.Time
}

type AttemptStart struct {
	TaskID, WorkerID string
	Number           int32
	At               time.Time
}

type Attempt struct {
	ID, TaskID, WorkerID   string
	Number                 int32
	Status                 task.AttemptStatus
	StartedAt, HeartbeatAt time.Time
}

type ClaimedTask struct {
	TaskID, WorkflowID, Key string
	Input                   workflow.Payload
	Attempt                 Attempt
	LeaseExpiresAt          time.Time
}

type Wakeup struct {
	WorkflowID string
	CreatedAt  time.Time
}

type Repository interface {
	Begin(context.Context) (Transaction, error)
}

type Transaction interface {
	LockNext(context.Context) (Candidate, error)
	Now(context.Context) (time.Time, error)
	InsertAttempt(context.Context, AttemptStart) (Attempt, error)
	UpdateTask(context.Context, ClaimedTask) error
	Enqueue(context.Context, string, time.Time) (Wakeup, error)
	IncrementVersion(context.Context, string) error
	Commit() error
	Rollback() error
}

type Service struct{ Repository Repository }

func (s Service) Claim(ctx context.Context, request ClaimRequest) (ClaimedTask, error) {
	// PostgreSQL timestamps have microsecond precision; do not silently shorten a lease.
	if request.WorkerID == "" || !utf8.ValidString(request.WorkerID) || strings.ContainsRune(request.WorkerID, 0) ||
		request.LeaseDuration <= 0 || request.LeaseDuration%time.Microsecond != 0 {
		return ClaimedTask{}, ErrInvalidRequest
	}
	tx, err := s.Repository.Begin(ctx)
	if err != nil {
		return ClaimedTask{}, err
	}
	defer tx.Rollback()
	candidate, err := tx.LockNext(ctx)
	if err != nil {
		return ClaimedTask{}, err
	}
	at, err := tx.Now(ctx)
	if err != nil {
		return ClaimedTask{}, err
	}
	if candidate.AvailableAt.After(at) {
		return ClaimedTask{}, ErrNoTask
	}
	next, err := task.Transition(candidate.State, task.Claim)
	if err != nil {
		return ClaimedTask{}, err
	}
	attempt, err := tx.InsertAttempt(ctx, AttemptStart{TaskID: candidate.ID, WorkerID: request.WorkerID, Number: next.AttemptCount, At: at})
	if err != nil {
		return ClaimedTask{}, err
	}
	claimed := ClaimedTask{TaskID: candidate.ID, WorkflowID: candidate.WorkflowID, Key: candidate.Key,
		Input: candidate.Input, Attempt: attempt, LeaseExpiresAt: at.Add(request.LeaseDuration)}
	if err := tx.UpdateTask(ctx, claimed); err != nil {
		return ClaimedTask{}, err
	}
	if _, err := tx.Enqueue(ctx, candidate.WorkflowID, at); err != nil {
		return ClaimedTask{}, err
	}
	if err := tx.IncrementVersion(ctx, candidate.WorkflowID); err != nil {
		return ClaimedTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return ClaimedTask{}, err
	}
	return claimed, nil
}
