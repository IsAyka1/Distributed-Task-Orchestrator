-- +goose Up
CREATE TABLE orchestrator.workflow_definitions (
    id uuid PRIMARY KEY,
    provider text COLLATE "C" NOT NULL CHECK (provider <> ''),
    name text COLLATE "C" NOT NULL CHECK (name <> ''),
    version bigint NOT NULL CHECK (version > 0),
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    created_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    CONSTRAINT workflow_definitions_identity UNIQUE (provider, name, version)
);

-- +goose StatementBegin
CREATE FUNCTION orchestrator.reject_definition_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'published workflow definitions are immutable'
        USING ERRCODE = 'check_violation';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER workflow_definitions_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON orchestrator.workflow_definitions
FOR EACH STATEMENT EXECUTE FUNCTION orchestrator.reject_definition_mutation();

CREATE TABLE orchestrator.workflow_runs (
    id uuid PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES orchestrator.workflow_definitions(id),
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    input jsonb NOT NULL DEFAULT 'null'::jsonb,
    output jsonb NOT NULL DEFAULT 'null'::jsonb,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT orchestrator.database_now(),
    started_at timestamptz,
    finished_at timestamptz,
    deadline_at timestamptz
);

CREATE INDEX workflow_runs_definition_id ON orchestrator.workflow_runs (definition_id);

-- +goose Down
DROP TABLE orchestrator.workflow_runs;
DROP TABLE orchestrator.workflow_definitions;
DROP FUNCTION orchestrator.reject_definition_mutation();
