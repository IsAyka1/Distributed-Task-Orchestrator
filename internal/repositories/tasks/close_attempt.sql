UPDATE orchestrator.task_attempts SET status = $2, finished_at = $3,
    error_type = NULLIF($4, ''), error_message = NULLIF($5, '') WHERE id = $1;
