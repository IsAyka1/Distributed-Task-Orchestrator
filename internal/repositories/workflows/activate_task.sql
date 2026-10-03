UPDATE orchestrator.task_runs
SET status = $2, input = COALESCE($3::jsonb, input), available_at = $4
WHERE id = $1;
