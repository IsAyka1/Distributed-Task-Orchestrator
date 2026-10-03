SELECT id, created_at, provider, name, version, definition
FROM orchestrator.workflow_definitions WHERE id = $1;
