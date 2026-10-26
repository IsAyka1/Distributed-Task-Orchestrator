package workflows

import (
	"context"
	"database/sql"
	"errors"

	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/jackc/pgx/v5/pgconn"
)

func (r Repository) BeginRead(ctx context.Context) (service.ReadTransaction, error) {
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return transaction{tx}, nil
}
func (t transaction) GetWorkflow(ctx context.Context, id string) (service.WorkflowDetails, error) {
	var run service.WorkflowDetails
	err := t.QueryRowContext(ctx, readWorkflowSQL, id).Scan(&run.ID, &run.DefinitionID, &run.Status,
		&run.Input.Value, &run.Output.Value, &run.Version, &run.CreatedAt, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return run, service.ErrWorkflowNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22021") {
		return run, service.ErrInvalidRequest
	}
	return run, err
}
func (t transaction) ListTasks(ctx context.Context, id string) ([]service.TaskDetails, error) {
	rows, err := t.QueryContext(ctx, readTasksSQL, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []service.TaskDetails
	for rows.Next() {
		var value service.TaskDetails
		if err := rows.Scan(&value.ID, &value.Key, &value.Status, &value.Input.Value, &value.Output.Value,
			&value.AttemptCount, &value.MaxAttemptCount, &value.CurrentAttemptID); err != nil {
			return nil, err
		}
		tasks = append(tasks, value)
	}
	return tasks, rows.Err()
}
