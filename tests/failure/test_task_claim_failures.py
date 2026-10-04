from concurrent.futures import ThreadPoolExecutor

from psycopg import sql
from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier


@pytest.mark.parametrize("stage", ["attempt", "task", "wakeup", "revision", "commit"])
def test_claim_failure_rolls_back_every_write(ready_execution, claims, stage):
    conn = ready_execution.connection
    before = ready_execution.snapshot()
    conn.execute("CREATE FUNCTION public.fail_claim() RETURNS trigger LANGUAGE plpgsql AS "
                 "'BEGIN RAISE EXCEPTION ''injected claim failure''; END;'")
    table, operation = {"attempt": ("task_attempts", "INSERT"), "task": ("task_runs", "UPDATE"),
                        "wakeup": ("workflow_wakeups", "INSERT"), "revision": ("workflow_runs", "UPDATE"),
                        "commit": ("task_attempts", "INSERT")}[stage]
    timing = "CONSTRAINT TRIGGER fail AFTER" if stage == "commit" else "TRIGGER fail BEFORE"
    deferred = "DEFERRABLE INITIALLY DEFERRED" if stage == "commit" else ""
    conn.execute(sql.SQL("CREATE {} {} ON orchestrator.{} {} FOR EACH ROW EXECUTE FUNCTION public.fail_claim()")
                 .format(sql.SQL(timing), sql.SQL(operation), sql.Identifier(table), sql.SQL(deferred)))
    result = claims.request()
    assert result.returncode != 0
    assert result.stdout == ""
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0
    conn.execute(sql.SQL("DROP TRIGGER fail ON orchestrator.{}").format(sql.Identifier(table)))
    assert claims.claim()["Attempt"]["Number"] == 1
    assert ready_execution.wakeups() == 1


def test_disconnected_claim_rolls_back_then_can_retry(ready_execution, claims, database):
    conn = ready_execution.connection
    before = ready_execution.snapshot()
    barrier = DatabaseBarrier(conn)
    barrier.pause_attempt_insert()
    with ThreadPoolExecutor(max_workers=1) as pool:
        with barrier.hold() as holder:
            pending = pool.submit(claims.request, worker="first", DATABASE_URL=make_conninfo(database.dsn, application_name="dying-claim"))
            pid = barrier.wait_blocked("dying-claim", holder)
            assert conn.execute("SELECT pg_terminate_backend(%s)", (pid,)).fetchone() == (True,)
            result = pending.result(timeout=8)
            assert result.returncode != 0
            assert result.stdout == ""
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0
    assert claims.claim()["Attempt"]["Number"] == 1
