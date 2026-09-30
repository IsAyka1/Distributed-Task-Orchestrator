-- +goose Up
CREATE TABLE orchestrator.task_runs (
    id uuid PRIMARY KEY,
    workflow_run_id uuid NOT NULL REFERENCES orchestrator.workflow_runs(id),
    task_key text COLLATE "C" NOT NULL CHECK (task_key <> ''),
    status text NOT NULL DEFAULT 'PENDING',
    input jsonb NOT NULL DEFAULT 'null'::jsonb,
    output jsonb NOT NULL DEFAULT 'null'::jsonb,
    max_attempt_count integer NOT NULL DEFAULT 1
        CONSTRAINT task_runs_single_attempt CHECK (max_attempt_count = 1),
    attempt_count integer NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0 AND attempt_count <= max_attempt_count),
    available_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    lease_owner text CHECK (lease_owner <> ''),
    lease_expires_at timestamptz,
    current_attempt_id uuid,
    created_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT task_runs_key UNIQUE (workflow_run_id, task_key),
    CONSTRAINT task_runs_lease_shape CHECK (
        (status = 'RUNNING' AND attempt_count > 0 AND current_attempt_id IS NOT NULL
            AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
        OR
        (status <> 'RUNNING' AND current_attempt_id IS NULL
            AND lease_owner IS NULL AND lease_expires_at IS NULL)
    )
);

CREATE TABLE orchestrator.task_attempts (
    id uuid PRIMARY KEY,
    task_run_id uuid NOT NULL REFERENCES orchestrator.task_runs(id),
    attempt_no integer NOT NULL
        CONSTRAINT task_attempts_single_attempt CHECK (attempt_no = 1),
    worker_id text NOT NULL CHECK (worker_id <> ''),
    status text NOT NULL DEFAULT 'RUNNING',
    started_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    heartbeat_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    finished_at timestamptz,
    error_type text,
    error_message text,
    CONSTRAINT task_attempts_number UNIQUE (task_run_id, attempt_no),
    CONSTRAINT task_attempts_owner UNIQUE (task_run_id, id)
);

ALTER TABLE orchestrator.task_runs ADD CONSTRAINT task_runs_current_attempt
    FOREIGN KEY (id, current_attempt_id)
    REFERENCES orchestrator.task_attempts (task_run_id, id);

CREATE TABLE orchestrator.workflow_wakeups (
    workflow_run_id uuid PRIMARY KEY REFERENCES orchestrator.workflow_runs(id),
    created_at timestamptz NOT NULL DEFAULT orchestrator.database_now()
);

CREATE INDEX task_runs_ready ON orchestrator.task_runs (available_at, id)
    WHERE status = 'READY';
CREATE INDEX task_runs_running ON orchestrator.task_runs (lease_expires_at)
    WHERE status = 'RUNNING';

-- +goose Down
DROP TABLE orchestrator.workflow_wakeups;
ALTER TABLE orchestrator.task_runs DROP CONSTRAINT task_runs_current_attempt;
DROP TABLE orchestrator.task_attempts;
DROP TABLE orchestrator.task_runs;
