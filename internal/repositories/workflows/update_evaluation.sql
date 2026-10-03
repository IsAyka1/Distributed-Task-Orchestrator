UPDATE orchestrator.workflow_runs
SET status = $2, output = $3, version = version + 1,
    started_at = CASE WHEN status = $4 THEN $5 ELSE started_at END,
    finished_at = CASE WHEN $2 IN ($6, $7) THEN $5 ELSE finished_at END
WHERE id = $1;
