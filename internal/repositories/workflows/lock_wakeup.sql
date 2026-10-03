SELECT workflow_run_id FROM orchestrator.workflow_wakeups
WHERE workflow_run_id = $1 FOR UPDATE;
