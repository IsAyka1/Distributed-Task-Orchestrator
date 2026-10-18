UPDATE orchestrator.task_runs SET status = $2, output = $3, finished_at = $4,
    current_attempt_id = NULL, lease_owner = NULL, lease_expires_at = NULL WHERE id = $1;
