SELECT id::text, created_at, definition
 FROM orchestrator.workflow_definitions WHERE name = $1 AND version = $2 AND provider = $3;
