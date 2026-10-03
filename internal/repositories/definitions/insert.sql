INSERT INTO orchestrator.workflow_definitions
 (id, provider, name, version, definition) VALUES (gen_random_uuid(), $1, $2, $3, $4)
 RETURNING id::text, created_at;
