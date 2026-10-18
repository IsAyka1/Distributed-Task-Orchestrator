from concurrent.futures import ThreadPoolExecutor

from psycopg import sql
from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier


@pytest.mark.parametrize("stage", ["attempt", "task", "wakeup", "revision", "commit"])
def test_completion_failure_rolls_back_all_writes(running_claim, completions, ready_execution, stage):
    conn = ready_execution.connection
    before = ready_execution.snapshot()
    conn.execute("CREATE FUNCTION public.fail_completion() RETURNS trigger LANGUAGE plpgsql AS "
                 "'BEGIN RAISE EXCEPTION ''injected completion failure''; END;'")
    table, operation = {"attempt": ("task_attempts", "UPDATE"), "task": ("task_runs", "UPDATE"),
                        "wakeup": ("workflow_wakeups", "INSERT"), "revision": ("workflow_runs", "UPDATE"),
                        "commit": ("task_attempts", "UPDATE")}[stage]
    timing = "CONSTRAINT TRIGGER fail AFTER" if stage == "commit" else "TRIGGER fail BEFORE"
    deferred = "DEFERRABLE INITIALLY DEFERRED" if stage == "commit" else ""
    conn.execute(sql.SQL("CREATE {} {} ON orchestrator.{} {} FOR EACH ROW EXECUTE FUNCTION public.fail_completion()")
                 .format(sql.SQL(timing), sql.SQL(operation), sql.Identifier(table), sql.SQL(deferred)))
    result = completions.request(running_claim, output={"n": 2})
    assert result.returncode != 0 and result.stdout == ""
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0
    conn.execute(sql.SQL("DROP TRIGGER fail ON orchestrator.{}").format(sql.Identifier(table)))
    completions.complete(running_claim, output={"n": 2})
    assert ready_execution.wakeups() == 1


def test_disconnected_completion_rolls_back(running_claim, completions, ready_execution, database):
    before = ready_execution.snapshot()
    barrier = DatabaseBarrier(ready_execution.connection)
    barrier.pause_completion()
    with ThreadPoolExecutor(max_workers=1) as pool:
        with barrier.hold() as holder:
            pending = pool.submit(completions.request, running_claim,
                                  DATABASE_URL=make_conninfo(database.dsn, application_name="dying-completion"))
            pid = barrier.wait_blocked("dying-completion", holder)
            assert ready_execution.connection.execute("SELECT pg_terminate_backend(%s)", (pid,)).fetchone() == (True,)
            result = pending.result(timeout=8)
            assert result.returncode != 0 and result.stdout == ""
    assert ready_execution.snapshot() == before
    assert ready_execution.wakeups() == 0
    completions.complete(running_claim)
