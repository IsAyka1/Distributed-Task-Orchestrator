SELECT t.status, a.status FROM orchestrator.task_runs t
JOIN orchestrator.workflow_runs w ON w.id=t.workflow_run_id
JOIN orchestrator.workflow_wakeups q ON q.workflow_run_id=w.id
JOIN orchestrator.task_attempts a ON a.id=t.current_attempt_id
WHERE t.id=$1 FOR UPDATE OF w, q, t, a NOWAIT;
