SELECT id, task_key, status, max_attempt_count, attempt_count, output
FROM orchestrator.task_runs WHERE workflow_run_id = $1 ORDER BY id FOR UPDATE;
