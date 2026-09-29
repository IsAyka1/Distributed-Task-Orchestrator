import json
from uuid import uuid4

import psycopg
from psycopg.types.json import Jsonb
import pytest


from support.schema import Definition, insert_run


def test_migration_and_repeated_run_preserve_definition_versions(application, connection):
    conn = connection
    v1 = Definition().insert(conn)
    run = insert_run(conn, v1)
    v2 = Definition(version=2, content='{"tasks":[{"id":"B"}]}').insert(conn)
    assert v1 != v2
    application.migrate()
    assert conn.execute(
        "SELECT d.version, d.definition FROM orchestrator.workflow_runs r "
        "JOIN orchestrator.workflow_definitions d ON d.id=r.definition_id WHERE r.id=%s", (run,),
    ).fetchone() == (1, json.loads(Definition().content))
    assert conn.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone() == (2,)


def test_definition_identity_is_provider_scoped_and_exact(connection):
    assert connection.execute(
        "SELECT pg_get_indexdef('orchestrator.workflow_definitions_identity'::regclass, n, true) "
        "FROM generate_series(1, 3) AS n ORDER BY n"
    ).fetchall() == [("name",), ("version",), ("provider",)]
    Definition().insert(connection)
    with pytest.raises(psycopg.errors.UniqueViolation):
        Definition().insert(connection)
    for definition in [Definition(provider="other"), Definition(provider="Demo"),
                       Definition(name="Sequence"), Definition(name="sequence "),
                       Definition(version=3)]:
        definition.insert(connection)


@pytest.mark.parametrize("definition", [
    Definition(provider=""), Definition(name=""), Definition(version=0), Definition(version=-1),
    Definition(content="null"), Definition(content="[]"), Definition(content='"text"'),
])
def test_invalid_definition_storage_values(connection, definition):
    with pytest.raises(psycopg.errors.CheckViolation):
        definition.insert(connection)


@pytest.mark.parametrize("statement", [
    "UPDATE orchestrator.workflow_definitions SET version=2",
    "UPDATE orchestrator.workflow_definitions SET definition='{}'",
    "DELETE FROM orchestrator.workflow_definitions",
    "TRUNCATE orchestrator.workflow_definitions CASCADE",
])
def test_published_definitions_cannot_be_rewritten(connection, statement):
    identifier = Definition().insert(connection)
    with pytest.raises(psycopg.errors.CheckViolation):
        connection.execute(statement)
    assert connection.execute(
        "SELECT version, definition FROM orchestrator.workflow_definitions WHERE id=%s", (identifier,),
    ).fetchone() == (1, json.loads(Definition().content))


def test_run_requires_an_existing_definition(connection):
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        insert_run(connection, uuid4())
    with pytest.raises(psycopg.errors.NotNullViolation):
        insert_run(connection, None)


@pytest.mark.parametrize("status", ["", "running", "UNKNOWN", "CANCELLED", "TIMED_OUT"])
def test_run_status_values_belong_to_application(connection, status):
    definition_id = Definition().insert(connection)
    run_id = insert_run(connection, definition_id, status=status)
    assert connection.execute(
        "SELECT status FROM orchestrator.workflow_runs WHERE id=%s", (run_id,),
    ).fetchone() == (status,)


@pytest.mark.parametrize("version", [0, -1])
def test_run_revision_must_be_positive(connection, version):
    definition_id = Definition().insert(connection)
    with pytest.raises(psycopg.errors.CheckViolation):
        insert_run(connection, definition_id, version=version)


@pytest.mark.parametrize("payload", [None, {"text": "Привет", "nested": [1, True, None]},
                                     [1, "two"], "text", 42, False])
def test_json_payload_round_trip(connection, payload):
    definition_id = Definition().insert(connection)
    run = insert_run(connection, definition_id, payload=payload)
    connection.execute(
        "UPDATE orchestrator.workflow_runs SET status='SUCCEEDED', output=%s, version=version+1 WHERE id=%s",
        (Jsonb(payload), run),
    )
    assert connection.execute(
        "SELECT input, output, version FROM orchestrator.workflow_runs WHERE id=%s", (run,),
    ).fetchone() == (payload, payload, 2)


def test_run_defaults_and_baseline_states(connection):
    definition_id = Definition(version=7).insert(connection)
    run = uuid4()
    connection.execute("INSERT INTO orchestrator.workflow_runs (id, definition_id) VALUES (%s, %s)",
                       (run, definition_id))
    assert connection.execute(
        "SELECT status, version, input, output, input IS NULL, output IS NULL "
        "FROM orchestrator.workflow_runs WHERE id=%s", (run,),
    ).fetchone() == ("PENDING", 1, None, None, False, False)
    for status in ["RUNNING", "SUCCEEDED", "FAILED"]:
        insert_run(connection, definition_id, status=status)


@pytest.mark.parametrize("column", ["input", "output", "status", "version"])
def test_run_required_values_reject_sql_null(connection, column):
    run = insert_run(connection, Definition().insert(connection))
    with pytest.raises(psycopg.errors.NotNullViolation):
        connection.execute(psycopg.sql.SQL("UPDATE orchestrator.workflow_runs SET {}=NULL WHERE id=%s")
                           .format(psycopg.sql.Identifier(column)), (run,))
