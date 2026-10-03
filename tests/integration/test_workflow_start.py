from copy import deepcopy
from uuid import UUID, uuid4

import pytest


def assert_no_execution(connection):
    assert connection.execute("""SELECT
        (SELECT count(*) FROM orchestrator.workflow_runs),
        (SELECT count(*) FROM orchestrator.task_runs),
        (SELECT count(*) FROM orchestrator.workflow_wakeups),
        (SELECT count(*) FROM orchestrator.task_attempts)""").fetchone() == (0, 0, 0, 0)


def test_start_materializes_selected_version_and_dependency_root(
        seed_definition, definitions, definition_data, workflows, connection, clock):
    published = seed_definition("sequence.yaml")
    newer = deepcopy(definition_data("sequence.yaml").document())
    newer["version"] = 2
    newer["tasks"] = [{"id": "other", "type": "activity"}]
    definitions.succeed("publish", newer)
    payload = {"n": [1, None, "value"]}
    run = workflows.start(published["id"], input=payload)
    assert UUID(run["id"]).version == 4
    assert run["status"] == "PENDING"
    assert run["created_at"] == clock.now.isoformat().replace("+00:00", "Z")
    assert connection.execute("""SELECT definition_id, status, input, output, version,
        created_at, started_at, finished_at, deadline_at FROM orchestrator.workflow_runs
        WHERE id = %s""", (run["id"],)).fetchone() == (
            UUID(published["id"]), "PENDING", payload, None, 1, clock.now, None, None, None)
    rows = connection.execute("""SELECT task_key, status, input, output, attempt_count,
        max_attempt_count, current_attempt_id, lease_owner, lease_expires_at,
        created_at, available_at, started_at, finished_at FROM orchestrator.task_runs
        WHERE workflow_run_id = %s ORDER BY task_key""", (run["id"],)).fetchall()
    assert rows == [(key, "PENDING", payload if key == "A" else None, None, 0, 1,
                     None, None, None, clock.now, clock.now, None, None) for key in ("A", "B", "C")]
    assert connection.execute("SELECT workflow_run_id, created_at FROM orchestrator.workflow_wakeups").fetchall() == [
        (UUID(run["id"]), clock.now)]
    assert connection.execute("SELECT count(*) FROM orchestrator.task_attempts").fetchone()[0] == 0
    # Baseline starts are deliberately not idempotent.
    second = workflows.start(published["id"], input=payload)
    assert second["id"] != run["id"]


@pytest.mark.parametrize("fields", [{}, {"input": None}, {"input": [3]}, {"input": "text"}, {"input": False},
                                  {"input": "🎯"}, {"input": 123456789012345678901234567890}])
def test_single_task_accepts_opaque_input(definitions, workflows, connection, fields):
    published = definitions.succeed("publish", {"provider": "demo", "name": "single", "version": 1,
        "tasks": [{"id": "A", "type": "activity"}]})
    run = workflows.start(published["id"], **fields)
    assert connection.execute("""SELECT w.input, t.input FROM orchestrator.workflow_runs w
        JOIN orchestrator.task_runs t ON t.workflow_run_id = w.id WHERE w.id = %s""",
        (run["id"],)).fetchone() == (fields.get("input"), fields.get("input"))


def test_unknown_definition_leaves_no_execution(workflows, connection):
    result = workflows.request(str(uuid4()))
    assert result.returncode != 0 and "definition_not_found" in result.stderr
    assert_no_execution(connection)


@pytest.mark.parametrize("kind, error", [("dag", "unsupported_workflow"), ("retry", "unsupported_policy")])
def test_unsupported_execution_leaves_no_rows(definitions, definition_data, workflows, connection, kind, error):
    document = definition_data("dag.yaml" if kind == "dag" else "sequence.yaml").document()
    if kind == "retry":
        document["tasks"][0]["max_attempts"] = 2
    published = definitions.succeed("publish", document)
    result = workflows.request(published["id"])
    assert result.returncode != 0 and error in result.stderr
    assert_no_execution(connection)


def test_invalid_stored_definition_cannot_start(workflows, connection):
    identifier = uuid4()
    connection.execute("""INSERT INTO orchestrator.workflow_definitions (id, name, provider, version, definition)
        VALUES (%s, 'corrupt', 'demo', 1, '{"tasks":[]}')""", (identifier,))
    result = workflows.request(str(identifier))
    assert result.returncode != 0 and "invalid stored definition" in result.stderr
    assert_no_execution(connection)


@pytest.mark.parametrize("payload", [r'"\u0000"', r'"\ud800"', '1e1000000'])
def test_jsonb_incompatible_payload_is_invalid_request(seed_definition, application, connection, payload):
    published = seed_definition("sequence.yaml")
    result = application.run("-workflow", "start", binary="probe", input=(
        '{"definition_id":"' + published["id"] + '","input":' + payload + '}'))
    assert result.returncode != 0 and "invalid_request" in result.stderr
    assert "SQLSTATE" not in result.stderr
    assert_no_execution(connection)
