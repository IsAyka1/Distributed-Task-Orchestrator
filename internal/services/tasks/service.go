package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var (
	ErrInvalidRequest = errors.New("invalid_request")
	ErrStaleAttempt   = errors.New("stale_attempt")
)

type CompletionRequest struct {
	TaskID, AttemptID, WorkerID string
	Event                       task.Event
	Output                      workflow.Payload
	ErrorType, ErrorMessage     string
}

type Snapshot struct {
	TaskID, WorkflowID, AttemptID, AttemptWorkerID string
	WorkflowStatus                                 workflow.Status
	State                                          task.State
	ActiveAttemptID, LeaseOwner                    *string
	LeaseExpiresAt                                 *time.Time
}

type Completion struct {
	TaskID, AttemptID       string
	State                   task.State
	Output                  workflow.Payload
	ErrorType, ErrorMessage string
	At                      time.Time
}

type Wakeup struct {
	WorkflowID string
	CreatedAt  time.Time
}

type Repository interface {
	Begin(context.Context) (Transaction, error)
}
type Transaction interface {
	LockAttempt(context.Context, CompletionRequest) (Snapshot, error)
	Now(context.Context) (time.Time, error)
	CloseAttempt(context.Context, Completion) error
	CompleteTask(context.Context, Completion) error
	Enqueue(context.Context, string, time.Time) (Wakeup, error)
	IncrementVersion(context.Context, string) error
	Commit() error
	Rollback() error
}

type Service struct{ Repository Repository }

func (s Service) CompleteAttempt(ctx context.Context, request CompletionRequest) error {
	request.Output.Value = slices.Clone(request.Output.Value)
	if len(request.Output.Value) == 0 {
		request.Output.Value = json.RawMessage("null")
	}
	if request.WorkerID == "" || !validText(request.WorkerID) || !validText(request.ErrorType) || !validText(request.ErrorMessage) ||
		!utf8.Valid(request.Output.Value) || !json.Valid(request.Output.Value) ||
		(request.Event != task.Succeed && request.Event != task.Fail) ||
		(request.Event == task.Succeed && (request.ErrorType != "" || request.ErrorMessage != "")) ||
		(request.Event == task.Fail && !bytes.Equal(bytes.TrimSpace(request.Output.Value), []byte("null"))) {
		return ErrInvalidRequest
	}
	tx, err := s.Repository.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := tx.LockAttempt(ctx, request)
	if err != nil {
		return err
	}
	at, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	if current.WorkflowStatus != workflow.Running || current.State.Status != task.Running ||
		current.State.LastAttempt.Status != task.AttemptRunning || current.ActiveAttemptID == nil ||
		*current.ActiveAttemptID != current.AttemptID || current.LeaseOwner == nil || *current.LeaseOwner != request.WorkerID ||
		current.AttemptWorkerID != request.WorkerID || current.LeaseExpiresAt == nil || !current.LeaseExpiresAt.After(at) {
		return ErrStaleAttempt
	}
	next, err := task.Transition(current.State, request.Event)
	if err != nil {
		return err
	}
	completion := Completion{TaskID: current.TaskID, AttemptID: current.AttemptID, State: next,
		Output: request.Output, ErrorType: request.ErrorType, ErrorMessage: request.ErrorMessage, At: at}
	if err := tx.CloseAttempt(ctx, completion); err != nil {
		return err
	}
	if err := tx.CompleteTask(ctx, completion); err != nil {
		return err
	}
	if _, err := tx.Enqueue(ctx, current.WorkflowID, at); err != nil {
		return err
	}
	if err := tx.IncrementVersion(ctx, current.WorkflowID); err != nil {
		return err
	}
	return tx.Commit()
}

func validText(value string) bool { return utf8.ValidString(value) && !strings.ContainsRune(value, 0) }
