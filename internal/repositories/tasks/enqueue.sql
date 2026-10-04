WITH inserted AS (
    INSERT INTO orchestrator.workflow_wakeups (workflow_run_id, created_at)
    VALUES ($1, $2) ON CONFLICT DO NOTHING
    RETURNING workflow_run_id, created_at
)
SELECT workflow_run_id, created_at FROM inserted
UNION ALL
SELECT workflow_run_id, created_at FROM orchestrator.workflow_wakeups
WHERE workflow_run_id = $1 AND NOT EXISTS (SELECT 1 FROM inserted);
