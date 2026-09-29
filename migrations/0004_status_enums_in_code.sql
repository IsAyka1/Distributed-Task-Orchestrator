-- +goose Up
ALTER TABLE orchestrator.workflow_runs DROP CONSTRAINT workflow_runs_status_check;
ALTER TABLE orchestrator.task_runs DROP CONSTRAINT task_runs_status_check;
ALTER TABLE orchestrator.task_attempts DROP CONSTRAINT task_attempts_status_check;

-- +goose Down
ALTER TABLE orchestrator.workflow_runs ADD CONSTRAINT workflow_runs_status_check
    CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED'));
ALTER TABLE orchestrator.task_runs ADD CONSTRAINT task_runs_status_check
    CHECK (status IN ('PENDING', 'READY', 'RUNNING', 'SUCCEEDED', 'FAILED'));
ALTER TABLE orchestrator.task_attempts ADD CONSTRAINT task_attempts_status_check
    CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED'));
