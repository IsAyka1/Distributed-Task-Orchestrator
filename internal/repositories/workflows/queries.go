package workflows

const insertWorkflowSQL = `INSERT INTO orchestrator.workflow_runs (id, definition_id, status, input)
VALUES (gen_random_uuid(), $1, $2, $3)
RETURNING id, status, created_at;`

const insertTaskSQL = `INSERT INTO orchestrator.task_runs
    (id, workflow_run_id, task_key, status, input, max_attempt_count, created_at, available_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $6)
RETURNING id, workflow_run_id, task_key, status, input, max_attempt_count,
    attempt_count, created_at, available_at;`

const enqueueSQL = `INSERT INTO orchestrator.workflow_wakeups (workflow_run_id, created_at) VALUES ($1, $2)
RETURNING workflow_run_id, created_at;`
