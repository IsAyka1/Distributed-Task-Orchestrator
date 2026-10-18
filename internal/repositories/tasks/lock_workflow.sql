SELECT id, status FROM orchestrator.workflow_runs
WHERE id = (SELECT workflow_run_id FROM orchestrator.task_runs WHERE id = $1)
FOR UPDATE;
