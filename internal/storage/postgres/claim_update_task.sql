UPDATE orchestrator.task_runs
SET status = $2, attempt_count = $3, current_attempt_id = $4,
    lease_owner = $5, lease_expires_at = $6, started_at = $7
WHERE id = $1;
