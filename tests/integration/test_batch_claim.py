from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta
import json

from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier


@pytest.mark.parametrize("limit", [1, 3, 100])
def test_batch_limit_order_and_committed_records(ready_batch, claims, clock, database, limit):
    conn = ready_batch[0].connection
    # Deliberately make workflow ID order disagree with availability/task ID order.
    rows = conn.execute("SELECT id FROM orchestrator.task_runs WHERE status='READY' "
                        "ORDER BY workflow_run_id").fetchall()
    for i, (task_id,) in enumerate(rows):
        conn.execute("UPDATE orchestrator.task_runs SET available_at=%s WHERE id=%s",
                     (clock.now - timedelta(seconds=i // 2), task_id))
    expected = conn.execute("SELECT id, workflow_run_id FROM orchestrator.task_runs WHERE status='READY' "
                            "ORDER BY available_at, id LIMIT %s", (limit,)).fetchall()
    claimed = claims.claim(limit=limit)
    assert [c["TaskID"] for c in claimed] == [str(row[0]) for row in expected]
    assert len({c["Attempt"]["ID"] for c in claimed}) == len(expected)
    with database.connect() as observer, observer.transaction():
        for value in claimed:
            assert value["Attempt"]["TaskID"] == value["TaskID"]
            assert value["Attempt"]["Number"] == 1
            assert datetime.fromisoformat(value["LeaseExpiresAt"]) == clock.now + timedelta(minutes=1)
            assert observer.execute("SELECT version FROM orchestrator.workflow_runs WHERE id=%s FOR UPDATE NOWAIT",
                                    (value["WorkflowID"],)).fetchone() == (3,)
            assert observer.execute("SELECT status, attempt_count, current_attempt_id::text, lease_owner "
                                    "FROM orchestrator.task_runs WHERE id=%s FOR UPDATE NOWAIT",
                                    (value["TaskID"],)).fetchone() == ("RUNNING", 1, value["Attempt"]["ID"], "worker")
            assert observer.execute("SELECT status FROM orchestrator.task_attempts WHERE id=%s FOR UPDATE NOWAIT",
                                    (value["Attempt"]["ID"],)).fetchone() == ("RUNNING",)
            assert observer.execute("SELECT count(*) FROM orchestrator.workflow_wakeups WHERE workflow_run_id=%s",
                                    (value["WorkflowID"],)).fetchone() == (1,)
    remaining = claims.claim(limit=100)
    assert len(claimed) + len(remaining) == 4
    assert claims.claim(limit=100) == []


def test_batch_empty_queue(claims):
    assert claims.claim(limit=3) == []


@pytest.mark.parametrize("limit", [-1, 0, 101, 2147483647])
def test_batch_rejects_invalid_limit_without_writes(ready_batch, claims, limit):
    before = [execution.snapshot() for execution in ready_batch]
    result = claims.request(limit=limit)
    assert result.returncode != 0
    assert result.stderr.strip() == "invalid_request"
    assert result.stdout == ""
    assert [execution.snapshot() for execution in ready_batch] == before
    assert all(execution.wakeups() == 0 for execution in ready_batch)


def test_batch_excludes_future_tasks_until_equality(ready_batch, claims, clock):
    conn = ready_batch[0].connection
    future_workflow = ready_batch[0].workflow_id
    conn.execute("UPDATE orchestrator.task_runs SET available_at=%s WHERE workflow_run_id=%s AND status='READY'",
                 (clock.now + timedelta(microseconds=1), future_workflow))
    claimed = claims.claim(limit=4)
    assert len(claimed) == 3
    assert future_workflow not in [c["WorkflowID"] for c in claimed]
    assert claims.claim(limit=4) == []
    clock.advance(timedelta(microseconds=1))
    assert [c["WorkflowID"] for c in claims.claim(limit=4)] == [future_workflow]


@pytest.mark.parametrize("table,column", [("workflow_runs", "id"), ("task_runs", "workflow_run_id")])
def test_batch_skips_locked_rows(ready_batch, claims, database, table, column):
    blocked = ready_batch[0].workflow_id
    with database.connect() as blocker, blocker.transaction():
        # Table and column come exclusively from the fixed parameter list above.
        blocker.execute(f"SELECT id FROM orchestrator.{table} WHERE {column}=%s FOR UPDATE", (blocked,))
        claimed = claims.claim(limit=4)
        assert len(claimed) == 3
        assert blocked not in [c["WorkflowID"] for c in claimed]
    assert [c["WorkflowID"] for c in claims.claim(limit=4)] == [blocked]


def test_competing_batches_do_not_share_attempts(ready_batch, claims, database):
    conn = ready_batch[0].connection
    barrier = DatabaseBarrier(conn)
    barrier.pause_attempt_insert()
    with ThreadPoolExecutor(max_workers=1) as pool:
        with barrier.hold() as holder:
            first = pool.submit(claims.request, limit=3, worker="first",
                                DATABASE_URL=make_conninfo(database.dsn, application_name="batch-first"))
            barrier.wait_blocked("batch-first", holder)
            assert conn.execute("SELECT count(*) FROM orchestrator.task_attempts").fetchone() == (0,)
            second = claims.claim(limit=3, worker="second")
            assert len(second) == 1
        result = first.result(timeout=8)
        assert result.returncode == 0, result.stderr
    claimed = json.loads(result.stdout) + second
    assert len(claimed) == len({c["TaskID"] for c in claimed}) == 4
    assert conn.execute("SELECT count(*), count(DISTINCT task_run_id) FROM orchestrator.task_attempts").fetchone() == (4, 4)


def test_batch_samples_one_lease_time_after_all_locks(ready_batch, claims, database, clock):
    conn = ready_batch[0].connection
    for execution in ready_batch:
        execution.signal()
    last = conn.execute("SELECT workflow_run_id FROM orchestrator.task_runs WHERE status='READY' "
                        "ORDER BY available_at DESC, id DESC LIMIT 1").fetchone()[0]
    with database.connect() as blocker, ThreadPoolExecutor(max_workers=1) as pool:
        with blocker.transaction():
            blocker.execute("SELECT workflow_run_id FROM orchestrator.workflow_wakeups WHERE workflow_run_id=%s FOR UPDATE",
                            (last,))
            pending = pool.submit(claims.claim, limit=4,
                                  DATABASE_URL=make_conninfo(database.dsn, application_name="batch-waiter"))
            DatabaseBarrier(conn).wait_blocked("batch-waiter", blocker.info.backend_pid)
            clock.advance(timedelta(hours=1))
        claimed = pending.result(timeout=8)
    assert len(claimed) == 4
    assert all(datetime.fromisoformat(c["Attempt"]["StartedAt"]) == clock.now for c in claimed), (clock.now, [c["Attempt"]["StartedAt"] for c in claimed])
    assert all(datetime.fromisoformat(c["LeaseExpiresAt"]) == clock.now + timedelta(minutes=1) for c in claimed)
