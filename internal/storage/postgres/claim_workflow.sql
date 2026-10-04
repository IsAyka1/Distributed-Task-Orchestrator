SELECT w.id FROM orchestrator.workflow_runs w
WHERE w.status = $1 AND EXISTS (
    SELECT 1 FROM orchestrator.task_runs t
    WHERE t.workflow_run_id = w.id AND t.status = $2
      AND t.available_at <= orchestrator.database_now()
      AND t.attempt_count < t.max_attempt_count
)
ORDER BY w.id
LIMIT 1 FOR UPDATE OF w SKIP LOCKED;
