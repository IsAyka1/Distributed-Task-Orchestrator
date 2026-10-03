package workflows

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/engine"
	definitionRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/jackc/pgx/v5/pgconn"
)

func (t transaction) LockWorkflow(ctx context.Context, id string) (service.EvaluationRun, error) {
	var run service.EvaluationRun
	err := t.QueryRowContext(ctx, lockWorkflowSQL, id).Scan(&run.ID, &run.DefinitionID, &run.Status, &run.Output.Value)
	if errors.Is(err, sql.ErrNoRows) {
		return run, service.ErrWorkflowNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22021") {
		return run, service.ErrInvalidRequest
	}
	return run, err
}

func (t transaction) LockWakeup(ctx context.Context, id string) (bool, error) {
	var found string
	err := t.QueryRowContext(ctx, lockWakeupSQL, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (t transaction) LockTasks(ctx context.Context, id string) ([]service.EvaluationTask, error) {
	rows, err := t.QueryContext(ctx, lockTasksSQL, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []service.EvaluationTask
	byID := make(map[string]int)
	for rows.Next() {
		var value service.EvaluationTask
		state := &value.Snapshot.State
		if err := rows.Scan(&value.ID, &value.Snapshot.Key, &state.Status, &state.MaxAttemptCount, &state.AttemptCount, &value.Output.Value); err != nil {
			return nil, err
		}
		byID[value.ID] = len(tasks)
		tasks = append(tasks, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Lock attempts only after every task lock has been acquired.
	attempts, err := t.QueryContext(ctx, lockAttemptsSQL, id)
	if err != nil {
		return nil, err
	}
	defer attempts.Close()
	for attempts.Next() {
		var taskID string
		var attempt task.Attempt
		if err := attempts.Scan(&taskID, &attempt.Number, &attempt.Status); err != nil {
			return nil, err
		}
		index, exists := byID[taskID]
		if !exists || tasks[index].Snapshot.State.LastAttempt != (task.Attempt{}) {
			return nil, engine.ErrInvalidState
		}
		tasks[index].Snapshot.State.LastAttempt = attempt
	}
	return tasks, attempts.Err()
}

func (t transaction) Now(ctx context.Context) (time.Time, error) {
	var at time.Time
	err := t.QueryRowContext(ctx, evaluationTimeSQL).Scan(&at)
	return at, err
}

func (t transaction) Activate(ctx context.Context, a service.Activation) error {
	_, err := t.ExecContext(ctx, activateTaskSQL, a.ID, task.Ready, []byte(a.Input.Value), a.At)
	return err
}

func (t transaction) UpdateEvaluation(ctx context.Context, run service.EvaluationRun, at time.Time) error {
	_, err := t.ExecContext(ctx, updateEvaluationSQL, run.ID, run.Status, []byte(run.Output.Value), workflow.Pending, at, workflow.Succeeded, workflow.Failed)
	return err
}

func (t transaction) DeleteWakeup(ctx context.Context, id string) error {
	_, err := t.ExecContext(ctx, deleteWakeupSQL, id)
	return err
}

func (t transaction) FindDefinition(ctx context.Context, id string) (definitions.PublishedDefinition, error) {
	return (definitionRepository.Repository{DB: t.Tx}).FindByID(ctx, id)
}
