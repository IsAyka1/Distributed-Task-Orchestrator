from dataclasses import asdict

import pytest

from support.definitions import TaskData
from support.workers import Workers


def test_worker_completes_chain_after_committing_claim(connection, seed_definition, workflows, application, clock):
    run = workflows.start(seed_definition("sequence.yaml")["id"], input={"n": 1})
    result = Workers(application, clock).step()
    assert [call["Key"] for call in result["Calls"]] == ["A", "B", "C"]
    assert [call["Input"]["Value"]["n"] for call in result["Calls"]] == [1, 2, 3]
    task_ids = [str(row[0]) for row in connection.execute(
        "SELECT id FROM orchestrator.task_runs WHERE workflow_run_id=%s ORDER BY task_key", (run["id"],))]
    assert [call["TaskID"] for call in result["Calls"]] == task_ids
    assert connection.execute("SELECT status, output FROM orchestrator.workflow_runs WHERE id=%s", (run["id"],)).fetchone() == (
        "SUCCEEDED", {"n": 4, "keys": task_ids})
    assert connection.execute("SELECT count(*) FROM orchestrator.task_attempts WHERE status='SUCCEEDED'").fetchone() == (3,)
    assert connection.execute("SELECT count(*) FROM orchestrator.task_runs WHERE current_attempt_id IS NOT NULL").fetchone() == (0,)
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_wakeups").fetchone() == (0,)
    assert result["Worked"] == [True, True, True, True, False]
    assert Workers(application, clock).step()["Calls"] == []


def test_unknown_activity_fails_without_external_call(connection, definitions, definition_data, workflows, application, clock):
    spec = definition_data("sequence.yaml")
    document = spec.document()
    document["tasks"] = [asdict(TaskData("unknown", "activity"))]
    published = definitions.succeed("publish", document)
    run = workflows.start(published["id"], input={"n": 1})
    assert Workers(application, clock).step()["Calls"] == []
    assert connection.execute("SELECT status FROM orchestrator.workflow_runs WHERE id=%s", (run["id"],)).fetchone() == ("FAILED",)
    assert connection.execute("SELECT status, error_type, error_message FROM orchestrator.task_attempts").fetchone() == (
        "FAILED", "unknown_activity", None)


@pytest.mark.parametrize("fail_at,expected", [("A", ["A"]), ("B", ["A", "B"]), ("C", ["A", "B", "C"])])
def test_activity_failure_stops_successors(connection, seed_definition, workflows, application, clock, fail_at, expected):
    run = workflows.start(seed_definition("sequence.yaml")["id"], input={"n": 1})
    result = Workers(application, clock).step(fail_at=fail_at)
    assert [call["Key"] for call in result["Calls"]] == expected
    assert connection.execute("SELECT status, output FROM orchestrator.workflow_runs WHERE id=%s", (run["id"],)).fetchone() == ("FAILED", None)
    assert connection.execute("SELECT error_type, error_message FROM orchestrator.task_attempts WHERE status='FAILED'").fetchone() == (
        "activity_failed", None)
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_wakeups").fetchone() == (0,)
