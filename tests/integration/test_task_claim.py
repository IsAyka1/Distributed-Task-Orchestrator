from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta
from uuid import UUID

from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier


def test_claim_returns_committed_attempt_and_lease(ready_execution, claims, clock, database):
    claimed = claims.claim(worker=" worker ")
    task_id, attempt = claimed["TaskID"], claimed["Attempt"]
    assert str(UUID(attempt["ID"])) == attempt["ID"]
    assert claimed["WorkflowID"] == ready_execution.workflow_id
    assert claimed["Key"] == "A"
    assert claimed["Input"]["Value"] == {"n": 1}
    assert attempt["TaskID"] == task_id
    assert attempt["Number"] == 1
    assert attempt["Status"] == "RUNNING"
    assert attempt["WorkerID"] == " worker "
    assert datetime.fromisoformat(attempt["StartedAt"]) == clock.now
    assert datetime.fromisoformat(attempt["HeartbeatAt"]) == clock.now
    assert datetime.fromisoformat(claimed["LeaseExpiresAt"]) == clock.now + timedelta(minutes=1)
    # A separate connection sees the committed records and can lock them immediately.
    with database.connect() as observer, observer.transaction():
        assert observer.execute("SELECT status, version FROM orchestrator.workflow_runs WHERE id=%s FOR UPDATE NOWAIT",
                                (claimed["WorkflowID"],)).fetchone() == ("RUNNING", 3)
        assert observer.execute("SELECT status, attempt_count, current_attempt_id, lease_owner, lease_expires_at, "
                                "started_at FROM orchestrator.task_runs WHERE id=%s FOR UPDATE NOWAIT",
                                (task_id,)).fetchone() == ("RUNNING", 1, UUID(attempt["ID"]), " worker ",
                                                          clock.now + timedelta(minutes=1), clock.now)
        assert observer.execute("SELECT task_run_id, attempt_no, worker_id, status, started_at, heartbeat_at "
                                "FROM orchestrator.task_attempts WHERE id=%s FOR UPDATE NOWAIT",
                                (attempt["ID"],)).fetchone() == (UUID(task_id), 1, " worker ", "RUNNING", clock.now, clock.now)
    before = ready_execution.snapshot()
    assert ready_execution.wakeups() == 1
    assert claims.claim() is None
    clock.advance(timedelta(minutes=1))
    assert claims.claim() is None  # Expiry does not enable recovery in stage 1.
    assert ready_execution.snapshot() == before


def test_future_availability_is_claimed_at_equality(ready_execution, claims, clock):
    ready_execution.connection.execute("UPDATE orchestrator.task_runs SET available_at=%s WHERE task_key='A'",
                                       (clock.now + timedelta(microseconds=1),))
    before = ready_execution.snapshot()
    assert claims.claim() is None
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0
    clock.advance(timedelta(microseconds=1))
    assert claims.claim()["Key"] == "A"


@pytest.mark.parametrize("mutation", [
    "UPDATE orchestrator.workflow_runs SET status='PENDING'",
    "UPDATE orchestrator.workflow_runs SET status='SUCCEEDED'",
    "UPDATE orchestrator.workflow_runs SET status='FAILED'",
    "UPDATE orchestrator.workflow_runs SET status='unknown'",
    "UPDATE orchestrator.task_runs SET status='PENDING'",
    "UPDATE orchestrator.task_runs SET status='SUCCEEDED'",
    "UPDATE orchestrator.task_runs SET status='FAILED'",
    "UPDATE orchestrator.task_runs SET status='unknown'",
    "UPDATE orchestrator.task_runs SET attempt_count=1",
])
def test_ineligible_work_is_unchanged(ready_execution, claims, mutation):
    ready_execution.connection.execute(mutation)
    before = ready_execution.snapshot()
    assert claims.claim() is None
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0


def test_empty_queue(claims):
    assert claims.claim() is None


@pytest.mark.parametrize("worker,duration", [("", "1m"), ("bad\x00worker", "1m"), ("worker", "0s"),
                                             ("worker", "-1s"), ("worker", "1ns"), ("worker", "1001ns")])
def test_invalid_claim_request_does_not_mutate(ready_execution, claims, worker, duration):
    before = ready_execution.snapshot()
    result = claims.request(worker=worker, duration=duration)
    assert result.returncode != 0
    assert result.stderr.strip() == "invalid_request"
    assert result.stdout == ""
    assert ready_execution.snapshot() == before


def test_existing_wakeup_is_preserved(ready_execution, claims, clock):
    ready_execution.signal()
    original = ready_execution.connection.execute("SELECT created_at FROM orchestrator.workflow_wakeups").fetchone()
    clock.advance(timedelta(seconds=2))
    assert claims.claim() is not None
    assert ready_execution.connection.execute("SELECT created_at FROM orchestrator.workflow_wakeups").fetchall() == [original]


@pytest.mark.parametrize("second_workflow", [False, True])
def test_competing_claim_skips_locked_workflow(ready_execution, claims, database, workflows, second_workflow):
    conn = ready_execution.connection
    if second_workflow:
        definition_id = conn.execute("SELECT definition_id FROM orchestrator.workflow_runs").fetchone()[0]
        other = workflows.start(str(definition_id))
        assert workflows.evaluate(other["id"]).returncode == 0
    barrier = DatabaseBarrier(conn)
    barrier.pause_attempt_insert()
    with ThreadPoolExecutor(max_workers=1) as pool:
        with barrier.hold() as holder:
            first = pool.submit(claims.request, worker="first", DATABASE_URL=make_conninfo(database.dsn, application_name="claim-first"))
            barrier.wait_blocked("claim-first", holder)
            assert conn.execute("SELECT count(*) FROM orchestrator.task_attempts").fetchone() == (0,)
            assert not first.done()
            second = claims.claim(worker="second")
            assert (second is not None) == second_workflow
        result = first.result(timeout=8)
        assert result.returncode == 0, result.stderr
    assert conn.execute("SELECT count(*), count(DISTINCT task_run_id) FROM orchestrator.task_attempts").fetchone() == (
        2 if second_workflow else 1, 2 if second_workflow else 1)
    assert claims.claim() is None


def test_locked_task_is_skipped(ready_execution, claims, database):
    with database.connect() as blocker, blocker.transaction():
        blocker.execute("SELECT id FROM orchestrator.task_runs WHERE task_key='A' FOR UPDATE")
        assert claims.claim() is None
    assert claims.claim() is not None


def test_lease_time_is_sampled_after_wakeup_lock_wait(ready_execution, claims, database, clock):
    ready_execution.signal()
    with database.connect() as blocker, ThreadPoolExecutor(max_workers=1) as pool:
        with blocker.transaction():
            blocker.execute("SELECT workflow_run_id FROM orchestrator.workflow_wakeups FOR UPDATE")
            pending = pool.submit(claims.claim, DATABASE_URL=make_conninfo(database.dsn, application_name="claim-waiter"))
            DatabaseBarrier(ready_execution.connection).wait_blocked("claim-waiter", blocker.info.backend_pid)
            clock.advance(timedelta(hours=1))
        result = pending.result(timeout=8)
    assert datetime.fromisoformat(result["Attempt"]["StartedAt"]) == clock.now
    assert datetime.fromisoformat(result["LeaseExpiresAt"]) == clock.now + timedelta(minutes=1)
