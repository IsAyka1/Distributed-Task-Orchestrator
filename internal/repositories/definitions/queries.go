package definitions

const insertSQL = `INSERT INTO orchestrator.workflow_definitions
 (id, provider, name, version, definition) VALUES (gen_random_uuid(), $1, $2, $3, $4)
 RETURNING id::text, created_at;`

const findSQL = `SELECT id::text, created_at, definition
 FROM orchestrator.workflow_definitions WHERE name = $1 AND version = $2 AND provider = $3;`

const findByIDSQL = `SELECT id, created_at, provider, name, version, definition
FROM orchestrator.workflow_definitions WHERE id = $1;`
