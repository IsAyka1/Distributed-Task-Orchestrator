package workflows

import (
	"context"
	"database/sql"
	_ "embed"
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

//go:embed insert_workflow.sql
var insertWorkflowSQL string

//go:embed insert_task.sql
var insertTaskSQL string

//go:embed enqueue.sql
var enqueueSQL string

func (r Repository) Begin(ctx context.Context) (service.Transaction, error) {
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	return transaction{tx}, nil
}

func (t transaction) InsertWorkflow(ctx context.Context, request service.StartRequest) (service.StartedWorkflow, error) {
	run := service.StartedWorkflow{Status: workflow.Pending}
	err := t.QueryRowContext(ctx, insertWorkflowSQL, request.DefinitionID, run.Status, []byte(request.Input.Value)).Scan(&run.ID, &run.CreatedAt)
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

func (t transaction) InsertTask(ctx context.Context, input service.TaskStart) error {
	_, err := t.ExecContext(ctx, insertTaskSQL, input.WorkflowID, input.Key, task.Pending,
		[]byte(input.Input.Value), input.MaxAttempts, input.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert workflow task: %w", err)
	}
	return nil
}

func (t transaction) Enqueue(ctx context.Context, run service.StartedWorkflow) error {
	_, err := t.ExecContext(ctx, enqueueSQL, run.ID, run.CreatedAt)
	if err != nil {
		return fmt.Errorf("enqueue workflow: %w", err)
	}
	return nil
}
