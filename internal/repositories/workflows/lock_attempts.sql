SELECT a.task_run_id, a.attempt_no, a.status
FROM orchestrator.task_attempts a
JOIN orchestrator.task_runs t ON t.id = a.task_run_id
WHERE t.workflow_run_id = $1 ORDER BY a.id FOR UPDATE OF a;
