from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from datetime import timedelta
from uuid import uuid4

from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier


@pytest.mark.parametrize("output", [{"n": 2}, [3], None])
def test_success_is_committed_and_propagated(running_claim, completions, ready_execution, clock, database, workflows, output):
    started = clock.now
    clock.advance(timedelta(seconds=2))
    completions.complete(running_claim, output=output)
    with database.connect() as observer, observer.transaction():
        assert observer.execute("SELECT status, output, current_attempt_id, lease_owner, lease_expires_at, "
                                "started_at, finished_at, attempt_count FROM orchestrator.task_runs "
                                "WHERE id=%s FOR UPDATE NOWAIT", (running_claim["TaskID"],)).fetchone() == (
                                    "SUCCEEDED", output, None, None, None, started, clock.now, 1)
        assert observer.execute("SELECT status, started_at, heartbeat_at, finished_at, error_type, error_message "
                                "FROM orchestrator.task_attempts FOR UPDATE NOWAIT").fetchone() == (
                                    "SUCCEEDED", started, started, clock.now, None, None)
        assert observer.execute("SELECT status, version FROM orchestrator.workflow_runs FOR UPDATE NOWAIT").fetchone() == ("RUNNING", 4)
        assert observer.execute("SELECT created_at FROM orchestrator.workflow_wakeups FOR UPDATE NOWAIT").fetchone() == (clock.now,)
    assert workflows.evaluate(ready_execution.workflow_id).returncode == 0
    assert ready_execution.connection.execute("SELECT status, input FROM orchestrator.task_runs WHERE task_key='B'").fetchone() == ("READY", output)


def test_failure_closes_attempt_without_output(running_claim, completions, ready_execution, workflows, clock):
    clock.advance(timedelta(seconds=1))
    completions.complete(running_claim, event="FAIL", error_type="permanent", error_message="Activity rejected request")
    conn = ready_execution.connection
    assert conn.execute("SELECT status, output, current_attempt_id, lease_owner, lease_expires_at, finished_at "
                        "FROM orchestrator.task_runs WHERE id=%s", (running_claim["TaskID"],)).fetchone() == (
                            "FAILED", None, None, None, None, clock.now)
    assert conn.execute("SELECT status, error_type, error_message, finished_at FROM orchestrator.task_attempts").fetchone() == (
        "FAILED", "permanent", "Activity rejected request", clock.now)
    assert ready_execution.wakeups() == 1
    assert workflows.evaluate(ready_execution.workflow_id).returncode == 0
    assert conn.execute("SELECT status, output FROM orchestrator.workflow_runs").fetchone() == ("FAILED", None)
    assert conn.execute("SELECT status FROM orchestrator.task_runs WHERE task_key <> 'A'").fetchall() == [("PENDING",), ("PENDING",)]


@pytest.mark.parametrize("change", ["token", "owner", "task", "other_task_token"])
def test_wrong_identity_is_rejected(running_claim, completions, ready_execution, claims, workflows, change):
    report = deepcopy(running_claim)
    if change == "token":
        report["Attempt"]["ID"] = str(uuid4())
    elif change == "owner":
        report["Attempt"]["WorkerID"] = "someone-else"
    elif change == "task":
        report["TaskID"] = str(uuid4())
    else:
        definition = ready_execution.connection.execute("SELECT definition_id FROM orchestrator.workflow_runs").fetchone()[0]
        other = workflows.start(str(definition))
        assert workflows.evaluate(other["id"]).returncode == 0
        report["Attempt"]["ID"] = claims.claim()["Attempt"]["ID"]
    before = ready_execution.snapshot()
    result = completions.request(report, output={"ignored": True})
    assert result.returncode != 0 and result.stderr.strip() == "stale_attempt"
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0


@pytest.mark.parametrize("mutation", [
    "UPDATE orchestrator.workflow_runs SET status='PENDING'",
    "UPDATE orchestrator.workflow_runs SET status='SUCCEEDED'",
    "UPDATE orchestrator.workflow_runs SET status='FAILED'",
    "UPDATE orchestrator.task_attempts SET status='SUCCEEDED'",
    "UPDATE orchestrator.task_attempts SET worker_id='other'",
    "UPDATE orchestrator.task_runs SET status='READY', current_attempt_id=NULL, lease_owner=NULL, lease_expires_at=NULL WHERE task_key='A'",
])
def test_nonrunning_or_inconsistent_report_is_rejected(running_claim, completions, ready_execution, mutation):
    ready_execution.connection.execute(mutation)
    before = ready_execution.snapshot()
    result = completions.request(running_claim)
    assert result.returncode != 0 and result.stderr.strip() == "stale_attempt"
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0


@pytest.mark.parametrize("delta,accepted", [(timedelta(minutes=1, microseconds=-1), True),
                                           (timedelta(minutes=1), False), (timedelta(minutes=1, microseconds=1), False)])
def test_strict_lease_boundary(running_claim, completions, ready_execution, clock, delta, accepted):
    before = ready_execution.snapshot()
    clock.advance(delta)
    result = completions.request(running_claim)
    assert (result.returncode == 0) == accepted
    if not accepted:
        assert result.stderr.strip() == "stale_attempt"
        assert ready_execution.snapshot() == before
        assert ready_execution.wakeups() == 0


@pytest.mark.parametrize("field", ["TaskID", "AttemptID"])
@pytest.mark.parametrize("value", ["bad-uuid", "", "bad\x00id"])
def test_malformed_ids_are_safe_errors(running_claim, completions, ready_execution, field, value):
    before = ready_execution.snapshot()
    report = deepcopy(running_claim)
    if field == "TaskID":
        report[field] = value
    else:
        report["Attempt"]["ID"] = value
    result = completions.request(report)
    assert result.returncode != 0 and result.stderr.strip() == "invalid_request"
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0


def test_postgresql_uuid_spellings_are_accepted(running_claim, completions):
    report = deepcopy(running_claim)
    report["TaskID"] = "{" + report["TaskID"].upper() + "}"
    report["Attempt"]["ID"] = report["Attempt"]["ID"].replace("-", "").upper()
    completions.complete(report)


@pytest.mark.parametrize("output", ["secret\x00value", "secret\ud800value", pytest.param(10 ** 131072, id="numeric-overflow")])
def test_unrepresentable_output_rolls_back(running_claim, completions, ready_execution, output):
    # PostgreSQL numeric limits are much larger than Python's default integer-to-string guard.
    import sys
    previous = sys.get_int_max_str_digits()
    sys.set_int_max_str_digits(0)
    try:
        before = ready_execution.snapshot()
        result = completions.request(running_claim, output=output)
        assert result.returncode != 0 and result.stderr.strip() == "invalid_request"
        assert ready_execution.snapshot() == before
        assert ready_execution.wakeups() == 0
    finally:
        sys.set_int_max_str_digits(previous)


def test_existing_wakeup_survives_completion(running_claim, completions, ready_execution, clock):
    ready_execution.signal()
    original = ready_execution.connection.execute("SELECT created_at FROM orchestrator.workflow_wakeups").fetchone()
    clock.advance(timedelta(seconds=1))
    completions.complete(running_claim)
    assert ready_execution.connection.execute("SELECT created_at FROM orchestrator.workflow_wakeups").fetchall() == [original]


@pytest.mark.parametrize("event", ["SUCCEED", "FAIL"])
def test_duplicate_report_does_not_change_result(running_claim, completions, ready_execution, clock, workflows, event):
    completions.complete(running_claim, event=event)
    assert workflows.evaluate(ready_execution.workflow_id).returncode == 0
    before = ready_execution.snapshot()
    clock.advance(timedelta(seconds=1))
    result = completions.request(running_claim, output={"overwrite": True})
    assert result.returncode != 0 and result.stderr.strip() == "stale_attempt"
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0


@pytest.mark.parametrize("table", ["workflow_runs", "workflow_wakeups", "task_runs", "task_attempts"])
def test_time_is_sampled_after_all_lock_waits(running_claim, completions, ready_execution, database, clock, table):
    ready_execution.signal()
    before = ready_execution.snapshot()
    with database.connect() as blocker, ThreadPoolExecutor(max_workers=1) as pool:
        with blocker.transaction():
            # Table names come exclusively from the fixed test parameter list.
            blocker.execute(f"SELECT * FROM orchestrator.{table} FOR UPDATE")
            pending = pool.submit(completions.request, running_claim,
                                  DATABASE_URL=make_conninfo(database.dsn, application_name="completion-waiter"))
            DatabaseBarrier(ready_execution.connection).wait_blocked("completion-waiter", blocker.info.backend_pid)
            clock.advance(timedelta(minutes=1))
        result = pending.result(timeout=8)
    assert result.returncode != 0 and result.stderr.strip() == "stale_attempt"
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 1


@pytest.mark.parametrize("competitor", ["complete", "evaluate"])
def test_completion_holds_workflow_lock_until_commit(running_claim, completions, ready_execution, database, workflows, competitor):
    barrier = DatabaseBarrier(ready_execution.connection)
    barrier.pause_completion()
    with ThreadPoolExecutor(max_workers=2) as pool:
        with barrier.hold() as holder:
            first = pool.submit(completions.request, running_claim, output={"n": 2},
                                DATABASE_URL=make_conninfo(database.dsn, application_name="first-completion"))
            first_pid = barrier.wait_blocked("first-completion", holder)
            if competitor == "complete":
                second = pool.submit(completions.request, running_claim, event="FAIL",
                                     DATABASE_URL=make_conninfo(database.dsn, application_name="second-writer"))
            else:
                second = pool.submit(workflows.evaluate, ready_execution.workflow_id,
                                     DATABASE_URL=make_conninfo(database.dsn, application_name="second-writer"))
            barrier.wait_blocked("second-writer", first_pid)
            assert not first.done() and not second.done()
        assert first.result(timeout=8).returncode == 0
        result = second.result(timeout=8)
    if competitor == "complete":
        assert result.returncode != 0 and result.stderr.strip() == "stale_attempt"
        assert ready_execution.wakeups() == 1
    else:
        assert result.returncode == 0, result.stderr
        assert ready_execution.wakeups() == 0
        assert ready_execution.connection.execute("SELECT status, input FROM orchestrator.task_runs WHERE task_key='B'").fetchone() == ("READY", {"n": 2})
    assert ready_execution.connection.execute("SELECT status, output FROM orchestrator.task_runs WHERE task_key='A'").fetchone() == ("SUCCEEDED", {"n": 2})


def test_evaluation_first_leaves_completion_wakeup(running_claim, completions, ready_execution, database, workflows):
    ready_execution.signal()
    barrier = DatabaseBarrier(ready_execution.connection)
    barrier.pause_wakeup_delete()
    with ThreadPoolExecutor(max_workers=2) as pool:
        with barrier.hold() as holder:
            evaluation = pool.submit(workflows.evaluate, ready_execution.workflow_id,
                                     DATABASE_URL=make_conninfo(database.dsn, application_name="first-evaluation"))
            evaluator_pid = barrier.wait_blocked("first-evaluation", holder)
            completion = pool.submit(completions.request, running_claim, output=[3],
                                     DATABASE_URL=make_conninfo(database.dsn, application_name="late-completion"))
            barrier.wait_blocked("late-completion", evaluator_pid)
        assert evaluation.result(timeout=8).returncode == 0
        assert completion.result(timeout=8).returncode == 0
    assert ready_execution.wakeups() == 1
    assert workflows.evaluate(ready_execution.workflow_id).returncode == 0
    assert ready_execution.connection.execute("SELECT status, input FROM orchestrator.task_runs WHERE task_key='B'").fetchone() == ("READY", [3])
