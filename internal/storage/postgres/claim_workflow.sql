SELECT w.id FROM orchestrator.workflow_runs w
CROSS JOIN LATERAL (
    SELECT t.id, t.available_at FROM orchestrator.task_runs t
    WHERE t.workflow_run_id = w.id AND t.status = $2
      AND t.available_at <= orchestrator.database_now()
      AND t.attempt_count < t.max_attempt_count
    ORDER BY t.available_at, t.id
    LIMIT 1
) candidate
WHERE w.status = $1
ORDER BY candidate.available_at, candidate.id
LIMIT $3 FOR UPDATE OF w SKIP LOCKED;
