SELECT id, task_key, input, status, max_attempt_count, attempt_count, available_at
FROM orchestrator.task_runs
WHERE workflow_run_id = $1 AND status = $2
  AND available_at <= orchestrator.database_now() AND attempt_count < max_attempt_count
ORDER BY id
LIMIT 1 FOR UPDATE SKIP LOCKED;
