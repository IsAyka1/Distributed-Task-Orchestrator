-- +goose Up
CREATE SCHEMA orchestrator;
CREATE FUNCTION orchestrator.database_now() RETURNS timestamptz
LANGUAGE sql VOLATILE AS 'SELECT clock_timestamp()';

-- +goose Down
DROP FUNCTION orchestrator.database_now();
DROP SCHEMA orchestrator;
