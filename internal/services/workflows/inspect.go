package workflows

import (
	"context"
	"errors"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type WorkflowDetails struct {
	ID, DefinitionID      string
	Status                workflow.Status
	Input, Output         workflow.Payload
	Version               int64
	CreatedAt             time.Time
	StartedAt, FinishedAt *time.Time
}
type TaskDetails struct {
	ID, Key                       string
	Status                        task.Status
	Input, Output                 workflow.Payload
	AttemptCount, MaxAttemptCount int32
	CurrentAttemptID              *string
}
type InspectionRepository interface {
	BeginRead(context.Context) (ReadTransaction, error)
}

// Reads share one read-only REPEATABLE READ snapshot, without execution locks.
type ReadTransaction interface {
	GetWorkflow(context.Context, string) (WorkflowDetails, error)
	FindDefinition(context.Context, string) (definitions.PublishedDefinition, error)
	ListTasks(context.Context, string) ([]TaskDetails, error)
	Commit() error
	Rollback() error
}
type Inspector struct{ Repository InspectionRepository }

func (s Inspector) GetWorkflow(ctx context.Context, id string) (WorkflowDetails, error) {
	run, _, err := s.inspect(ctx, id, false)
	return run, err
}
func (s Inspector) ListTasks(ctx context.Context, id string) ([]TaskDetails, error) {
	_, tasks, err := s.inspect(ctx, id, true)
	return tasks, err
}
func (s Inspector) inspect(ctx context.Context, id string, includeTasks bool) (WorkflowDetails, []TaskDetails, error) {
	tx, err := s.Repository.BeginRead(ctx)
	if err != nil {
		return WorkflowDetails{}, nil, err
	}
	defer tx.Rollback()
	run, err := tx.GetWorkflow(ctx, id)
	if err != nil {
		return WorkflowDetails{}, nil, err
	}
	switch run.Status {
	case workflow.Pending, workflow.Running, workflow.Succeeded, workflow.Failed:
	default:
		return WorkflowDetails{}, nil, errInvalidInspection
	}
	var tasks []TaskDetails
	if includeTasks {
		published, err := tx.FindDefinition(ctx, run.DefinitionID)
		if errors.Is(err, definitions.ErrDefinitionNotFound) || errors.Is(err, definitions.ErrInvalidDefinitionID) || errors.Is(err, workflow.ErrInvalidDefinition) {
			return WorkflowDetails{}, nil, errInvalidInspection
		}
		if err != nil {
			return WorkflowDetails{}, nil, err
		}
		tasks, err = tx.ListTasks(ctx, run.ID)
		if err != nil {
			return WorkflowDetails{}, nil, err
		}
		tasks, err = orderTasks(published.Definition, tasks)
		if err != nil {
			return WorkflowDetails{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkflowDetails{}, nil, err
	}
	return run, tasks, nil
}

var errInvalidInspection = errors.New("inconsistent workflow inspection snapshot")

func orderTasks(definition *workflow.Definition, tasks []TaskDetails) ([]TaskDetails, error) {
	if definition == nil || definition.ValidateSequence() != nil {
		return nil, errInvalidInspection
	}
	steps := definition.Tasks()
	if len(steps) != len(tasks) {
		return nil, errInvalidInspection
	}
	byKey := make(map[string]TaskDetails, len(tasks))
	for _, value := range tasks {
		switch value.Status {
		case task.Pending, task.Ready, task.Running, task.Succeeded, task.Failed:
		default:
			return nil, errInvalidInspection
		}
		byKey[value.Key] = value
	}
	if len(byKey) != len(tasks) {
		return nil, errInvalidInspection
	}
	next := make(map[string]string, len(steps))
	var key string
	for _, step := range steps {
		if len(step.DependsOn) == 0 {
			key = step.ID
		} else {
			next[step.DependsOn[0]] = step.ID
		}
	}
	ordered := make([]TaskDetails, 0, len(tasks))
	for range steps {
		value, exists := byKey[key]
		if !exists {
			return nil, errInvalidInspection
		}
		ordered = append(ordered, value)
		key = next[key]
	}
	return ordered, nil
}
