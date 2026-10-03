package workflows

import (
	"context"
	"errors"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/engine"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var ErrWorkflowNotFound = errors.New("workflow_not_found")

type EvaluationRun struct {
	ID, DefinitionID string
	Status           workflow.Status
	Output           workflow.Payload
}

type EvaluationTask struct {
	ID       string
	Snapshot engine.TaskSnapshot
	Output   workflow.Payload
}

type Activation struct {
	ID    string
	Input workflow.Payload
	At    time.Time
}

type EvaluationTransaction interface {
	FindDefinition(context.Context, string) (definitions.PublishedDefinition, error)
	LockWorkflow(context.Context, string) (EvaluationRun, error)
	LockWakeup(context.Context, string) (bool, error)
	LockTasks(context.Context, string) ([]EvaluationTask, error)
	Now(context.Context) (time.Time, error)
	Activate(context.Context, Activation) error
	UpdateEvaluation(context.Context, EvaluationRun, time.Time) error
	DeleteWakeup(context.Context, string) error
}

func (s Service) EvaluateWorkflow(ctx context.Context, id string) error {
	tx, err := s.Repository.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	run, err := tx.LockWorkflow(ctx, id)
	if err != nil {
		return err
	}
	pending, err := tx.LockWakeup(ctx, run.ID)
	if err != nil || !pending {
		return err
	}
	if run.Status != workflow.Succeeded && run.Status != workflow.Failed {
		if err := evaluateLocked(ctx, tx, run); err != nil {
			return err
		}
	}
	if err := tx.DeleteWakeup(ctx, run.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func evaluateLocked(ctx context.Context, tx Transaction, run EvaluationRun) error {
	published, err := tx.FindDefinition(ctx, run.DefinitionID)
	if err != nil {
		return err
	}
	tasks, err := tx.LockTasks(ctx, run.ID)
	if err != nil {
		return err
	}
	snapshots := make([]engine.TaskSnapshot, 0, len(tasks))
	byKey := make(map[string]EvaluationTask, len(tasks))
	for _, t := range tasks {
		snapshots = append(snapshots, t.Snapshot)
		byKey[t.Snapshot.Key] = t
	}
	changes, err := engine.Evaluate(*published.Definition, run.Status, snapshots)
	if err != nil {
		return err
	}
	if changes.Status == run.Status && len(changes.Tasks) == 0 {
		return nil
	}
	at, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	definitions := published.Definition.Tasks()
	hasSuccessor := make(map[string]bool, len(definitions))
	for _, d := range definitions {
		if len(d.DependsOn) > 0 {
			hasSuccessor[d.DependsOn[0]] = true
		}
		for _, changed := range changes.Tasks {
			if changed.Key != d.ID {
				continue
			}
			activation := Activation{ID: byKey[d.ID].ID, At: at}
			// A nil input preserves the root input fixed by StartWorkflow.
			if len(d.DependsOn) > 0 {
				activation.Input = byKey[d.DependsOn[0]].Output
			}
			if err := tx.Activate(ctx, activation); err != nil {
				return err
			}
		}
	}
	run.Status = changes.Status
	if run.Status == workflow.Succeeded {
		for _, d := range definitions {
			if !hasSuccessor[d.ID] {
				run.Output = byKey[d.ID].Output
			}
		}
	}
	return tx.UpdateEvaluation(ctx, run, at)
}
