package worker

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
)

type Queue interface {
	Claim(context.Context, queue.ClaimRequest) (queue.ClaimedTask, error)
}
type Completer interface {
	CompleteAttempt(context.Context, tasks.CompletionRequest) error
}
type Evaluator interface {
	EvaluateWorkflow(context.Context, string) error
}
type Repository interface {
	NextWakeup(context.Context) (string, error)
}
type Config struct {
	WorkerID                                      string
	PollInterval, OperationTimeout, LeaseDuration time.Duration
}
type Runner struct {
	Queue      Queue
	Completer  Completer
	Evaluator  Evaluator
	Repository Repository
	Registry   Registry
	Clock      Clock
	Config     Config
}

func (r Runner) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		worked, err := r.Step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !worked {
			if err := r.Clock.Wait(ctx, r.Config.PollInterval); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
	return nil
}

// Step processes at most one signal and one committed claim.
func (r Runner) Step(ctx context.Context) (bool, error) {
	if r.Config.WorkerID == "" || !utf8.ValidString(r.Config.WorkerID) || strings.ContainsRune(r.Config.WorkerID, 0) ||
		r.Config.PollInterval <= 0 || r.Config.OperationTimeout <= 0 || r.Config.LeaseDuration <= r.Config.OperationTimeout || r.Config.LeaseDuration%time.Microsecond != 0 {
		return false, errors.New("invalid worker configuration")
	}
	operation, cancel := r.Clock.WithTimeout(ctx, r.Config.OperationTimeout)
	id, err := r.Repository.NextWakeup(operation)
	if err == nil && id != "" {
		err = r.Evaluator.EvaluateWorkflow(operation, id)
	}
	cancel()
	if err != nil {
		return false, err
	}
	worked, err := r.executeOne(ctx)
	return worked || id != "", err
}

func (r Runner) executeOne(ctx context.Context) (bool, error) {
	// Start the budget before claim, so lock/commit latency consumes activity time.
	// Reserve one operation timeout for completion; database fencing stays authoritative.
	activityCtx, stop := r.Clock.WithTimeout(ctx, r.Config.LeaseDuration-r.Config.OperationTimeout)
	defer stop()
	claimCtx, cancel := r.Clock.WithTimeout(activityCtx, r.Config.OperationTimeout)
	claimed, err := r.Queue.Claim(claimCtx, queue.ClaimRequest{WorkerID: r.Config.WorkerID, LeaseDuration: r.Config.LeaseDuration})
	cancel()
	if errors.Is(err, queue.ErrNoTask) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	report := tasks.CompletionRequest{TaskID: claimed.TaskID, AttemptID: claimed.Attempt.ID, WorkerID: r.Config.WorkerID, Event: task.Fail}
	activity := r.Registry.activities[claimed.Key]
	switch {
	case activityCtx.Err() != nil:
		report.ErrorType = "activity_timeout"
	case activity == nil:
		report.ErrorType = "unknown_activity"
	default:
		output, activityErr := activity.Execute(activityCtx, claimed.Input, claimed.TaskID)
		switch {
		case activityCtx.Err() != nil:
			report.ErrorType = "activity_timeout"
		case activityErr != nil:
			report.ErrorType = "activity_failed"
		default:
			report.Event = task.Succeed
			report.Output = output
		}
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	completionCtx, cancel := r.Clock.WithTimeout(ctx, r.Config.OperationTimeout)
	defer cancel()
	return true, r.Completer.CompleteAttempt(completionCtx, report)
}
