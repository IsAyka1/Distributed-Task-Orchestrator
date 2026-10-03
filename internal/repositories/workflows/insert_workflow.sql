INSERT INTO orchestrator.workflow_runs (id, definition_id, status, input)
VALUES (gen_random_uuid(), $1, $2, $3)
RETURNING id, created_at;
