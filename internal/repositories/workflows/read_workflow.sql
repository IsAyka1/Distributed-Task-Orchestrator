SELECT id, definition_id, status, input, output, version, created_at, started_at, finished_at
FROM orchestrator.workflow_runs WHERE id = $1;
