SELECT id, worker_id, attempt_no, status FROM orchestrator.task_attempts
WHERE task_run_id = $1 AND id = $2 FOR UPDATE;
