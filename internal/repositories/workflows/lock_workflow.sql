SELECT id, definition_id, status, output
FROM orchestrator.workflow_runs WHERE id = $1 FOR UPDATE;
