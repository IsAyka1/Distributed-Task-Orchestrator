from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta
from uuid import uuid4

from psycopg.conninfo import make_conninfo
import pytest

from conftest import INITIAL_TIME
from support.evaluation import DatabaseBarrier, Execution


@pytest.fixture
def execution(connection, seed_definition, workflows):
    run = workflows.start(seed_definition("sequence.yaml")["id"], input={"n": 1})
    return Execution(connection, run["id"])


def evaluate(workflows, execution, **env):
    result = workflows.evaluate(execution.workflow_id, **env)
    assert result.returncode == 0, result.stderr


def test_evaluation_activates_root_once(execution, workflows, clock):
    clock.advance(timedelta(seconds=10))
    evaluate(workflows, execution)
    assert execution.connection.execute(
        "SELECT status, version, started_at, finished_at, output FROM orchestrator.workflow_runs WHERE id=%s",
        (execution.workflow_id,)).fetchone() == ("RUNNING", 2, clock.now, None, None)
    assert execution.connection.execute(
        "SELECT task_key, status, input, available_at, started_at FROM orchestrator.task_runs ORDER BY task_key"
    ).fetchall() == [("A", "READY", {"n": 1}, clock.now, None),
                    ("B", "PENDING", None, INITIAL_TIME, None), ("C", "PENDING", None, INITIAL_TIME, None)]
    before = execution.snapshot()
    clock.advance(timedelta(seconds=10))
    evaluate(workflows, execution)  # Absent wakeup.
    execution.signal()
    evaluate(workflows, execution)  # READY frontier.
    assert execution.snapshot() == before
    assert execution.wakeups() == 0


@pytest.mark.parametrize("final_output", [None, {"answer": [1, 2]}, [3], "done"])
def test_evaluation_advances_chain_and_preserves_terminal_result(execution, workflows, clock, final_output):
    evaluate(workflows, execution)
    for key, output in [("A", {"n": 2}), ("B", [3]), ("C", final_output)]:
        execution.claim(key)
        execution.complete(key, output)
        clock.advance(timedelta(seconds=1))
        evaluate(workflows, execution)
    assert execution.connection.execute(
        "SELECT status, version, output, started_at, finished_at FROM orchestrator.workflow_runs WHERE id=%s",
        (execution.workflow_id,)).fetchone() == ("SUCCEEDED", 11, final_output, INITIAL_TIME, clock.now)
    assert execution.connection.execute("SELECT task_key, input FROM orchestrator.task_runs ORDER BY task_key").fetchall() == [
        ("A", {"n": 1}), ("B", {"n": 2}), ("C", [3])]
    before = execution.snapshot()
    clock.advance(timedelta(days=1))
    execution.signal()
    evaluate(workflows, execution)
    assert execution.snapshot() == before
    assert execution.wakeups() == 0


def test_evaluation_failure_leaves_successors_pending(execution, workflows, clock):
    evaluate(workflows, execution)
    execution.claim("A")
    execution.complete("A", status="FAILED")
    clock.advance(timedelta(seconds=2))
    evaluate(workflows, execution)
    assert execution.connection.execute("SELECT status, output, finished_at FROM orchestrator.workflow_runs").fetchone() == (
        "FAILED", None, clock.now)
    assert execution.connection.execute("SELECT status FROM orchestrator.task_runs ORDER BY task_key").fetchall() == [
        ("FAILED",), ("PENDING",), ("PENDING",)]
    before = execution.snapshot()
    execution.signal()
    evaluate(workflows, execution)
    assert execution.snapshot() == before
    assert execution.wakeups() == 0


@pytest.mark.parametrize("mutation", [
    "DELETE FROM orchestrator.task_runs WHERE task_key='B'",
    "UPDATE orchestrator.task_runs SET status='READY' WHERE task_key='B'",
    "UPDATE orchestrator.task_runs SET status='SUCCEEDED', attempt_count=1 WHERE task_key='A'",
    "UPDATE orchestrator.workflow_runs SET status='unknown'",
])
def test_invalid_snapshot_keeps_signal_and_state(execution, workflows, mutation):
    execution.connection.execute(mutation)
    before = execution.snapshot()
    result = workflows.evaluate(execution.workflow_id)
    assert result.returncode != 0
    assert execution.snapshot() == before
    assert execution.wakeups() == 1


@pytest.mark.parametrize("identifier,code", [(str(uuid4()), "workflow_not_found"), ("bad", "invalid_request"), ("\x00", "invalid_request")])
def test_evaluation_missing_or_invalid_id(workflows, identifier, code):
    result = workflows.evaluate(identifier)
    assert result.returncode != 0
    assert result.stderr.strip() == code


def named_evaluation(workflows, execution, database, name):
    return workflows.evaluate(execution.workflow_id, DATABASE_URL=make_conninfo(database.dsn, application_name=name))


def test_competing_evaluations_consume_once(execution, workflows, database):
    barrier = DatabaseBarrier(execution.connection)
    barrier.pause_wakeup_delete()
    with ThreadPoolExecutor(max_workers=2) as pool:
        with barrier.hold() as holder:
            first = pool.submit(named_evaluation, workflows, execution, database, "evaluate-first")
            first_pid = barrier.wait_blocked("evaluate-first", holder)
            second = pool.submit(named_evaluation, workflows, execution, database, "evaluate-second")
            barrier.wait_blocked("evaluate-second", first_pid)
        for future in (first, second):
            result = future.result(timeout=8)
            assert result.returncode == 0, result.stderr
    assert execution.connection.execute("SELECT status, version FROM orchestrator.workflow_runs").fetchone() == ("RUNNING", 2)
    assert execution.wakeups() == 0


@pytest.mark.parametrize("first", ["completion", "evaluation"])
def test_completion_evaluation_race_preserves_signal(execution, workflows, database, first):
    evaluate(workflows, execution)
    execution.claim("A")
    execution.signal()
    barrier = DatabaseBarrier(execution.connection)
    with database.connect() as writer, ThreadPoolExecutor(max_workers=2) as pool:
        writer.execute("SET application_name = 'complete'")
        completion = Execution(writer, execution.workflow_id)
        if first == "completion":
            with writer.transaction():
                completion.lock()
                completion.complete("A", {"result": True})
                future = pool.submit(named_evaluation, workflows, execution, database, "evaluate")
                barrier.wait_blocked("evaluate", writer.info.backend_pid)
            result = future.result(timeout=8)
            assert result.returncode == 0, result.stderr
            assert execution.wakeups() == 0
        else:
            barrier.pause_wakeup_delete()
            with barrier.hold() as holder:
                future = pool.submit(named_evaluation, workflows, execution, database, "evaluate")
                evaluator = barrier.wait_blocked("evaluate", holder)
                finish = pool.submit(completion.complete, "A", {"result": True})
                barrier.wait_blocked("complete", evaluator)
            result = future.result(timeout=8)
            assert result.returncode == 0, result.stderr
            finish.result(timeout=8)
            assert execution.wakeups() == 1
            evaluate(workflows, execution)
        assert execution.connection.execute("SELECT status, input FROM orchestrator.task_runs WHERE task_key='B'").fetchone() == (
            "READY", {"result": True})
        assert execution.wakeups() == 0


def test_evaluation_samples_time_after_waiting_for_locks(execution, workflows, database, clock):
    with database.connect() as blocker, ThreadPoolExecutor(max_workers=1) as pool:
        with blocker.transaction():
            blocker.execute("SELECT id FROM orchestrator.task_runs WHERE workflow_run_id=%s ORDER BY id FOR UPDATE",
                            (execution.workflow_id,))
            future = pool.submit(named_evaluation, workflows, execution, database, "evaluate")
            DatabaseBarrier(execution.connection).wait_blocked("evaluate", blocker.info.backend_pid)
            clock.advance(timedelta(hours=1))
        result = future.result(timeout=8)
        assert result.returncode == 0, result.stderr
    assert execution.connection.execute("SELECT started_at FROM orchestrator.workflow_runs").fetchone() == (clock.now,)
    assert execution.connection.execute("SELECT available_at FROM orchestrator.task_runs WHERE task_key='A'").fetchone() == (clock.now,)


def test_evaluation_uses_one_database_connection(execution, application):
    import json
    result = application.run("-workflow", "evaluate-one-connection", binary="probe",
                             input=json.dumps(dict(workflow_id=execution.workflow_id)))
    assert result.returncode == 0, result.stderr
    assert execution.wakeups() == 0
