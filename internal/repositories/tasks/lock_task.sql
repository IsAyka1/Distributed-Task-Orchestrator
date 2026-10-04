SELECT id, status, max_attempt_count, attempt_count, current_attempt_id, lease_owner, lease_expires_at
FROM orchestrator.task_runs WHERE id = $1 AND workflow_run_id = $2 FOR UPDATE;
