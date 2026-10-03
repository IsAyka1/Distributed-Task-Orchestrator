package workflows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
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

func (t transaction) InsertWorkflow(ctx context.Context, request service.StartRequest) (service.StartedWorkflow, error) {
	var run service.StartedWorkflow
	err := t.QueryRowContext(ctx, insertWorkflowSQL, request.DefinitionID, workflow.Pending, []byte(request.Input.Value)).Scan(&run.ID, &run.Status, &run.CreatedAt)
	if err != nil {
		// JSONB has stricter Unicode and numeric limits than JSON syntax.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "22P05" || pgErr.Code == "22P02" || pgErr.Code == "22003") {
			return service.StartedWorkflow{}, fmt.Errorf("%w: input is not representable as JSONB", service.ErrInvalidRequest)
		}
		return service.StartedWorkflow{}, fmt.Errorf("insert workflow: %w", err)
	}
	return run, nil
}

func (t transaction) InsertTask(ctx context.Context, input service.TaskStart) (service.StartedTask, error) {
	var created service.StartedTask
	err := t.QueryRowContext(ctx, insertTaskSQL, input.WorkflowID, input.Key, task.Pending,
		[]byte(input.Input.Value), input.MaxAttempts, input.CreatedAt).Scan(
		&created.ID, &created.WorkflowID, &created.Key, &created.Status, &created.Input.Value,
		&created.MaxAttempts, &created.AttemptCount, &created.CreatedAt, &created.AvailableAt)
	if err != nil {
		return service.StartedTask{}, fmt.Errorf("insert workflow task: %w", err)
	}
	return created, nil
}

func (t transaction) Enqueue(ctx context.Context, run service.StartedWorkflow) (service.Wakeup, error) {
	var wakeup service.Wakeup
	err := t.QueryRowContext(ctx, enqueueSQL, run.ID, run.CreatedAt).Scan(&wakeup.WorkflowID, &wakeup.CreatedAt)
	if err != nil {
		return service.Wakeup{}, fmt.Errorf("enqueue workflow: %w", err)
	}
	return wakeup, nil
}
