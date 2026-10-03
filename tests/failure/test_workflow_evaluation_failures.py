from concurrent.futures import ThreadPoolExecutor

from psycopg import sql
from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier, Execution


@pytest.mark.parametrize("stage", ["activation", "revision", "delete", "commit"])
def test_evaluation_failure_rolls_back_state_and_signal(connection, seed_definition, workflows, stage):
    run = workflows.start(seed_definition("sequence.yaml")["id"])
    execution = Execution(connection, run["id"])
    before = execution.snapshot()
    connection.execute("CREATE FUNCTION public.fail_evaluation() RETURNS trigger LANGUAGE plpgsql AS "
                       "'BEGIN RAISE EXCEPTION ''injected failure''; END;'")
    table, operation = {"activation": ("task_runs", "UPDATE"), "revision": ("workflow_runs", "UPDATE"),
                        "delete": ("workflow_wakeups", "DELETE"), "commit": ("workflow_wakeups", "DELETE")}[stage]
    timing = "CONSTRAINT TRIGGER fail AFTER" if stage == "commit" else "TRIGGER fail BEFORE"
    deferred = "DEFERRABLE INITIALLY DEFERRED" if stage == "commit" else ""
    connection.execute(sql.SQL("CREATE {} {} ON orchestrator.{} {} FOR EACH ROW EXECUTE FUNCTION public.fail_evaluation()")
                       .format(sql.SQL(timing), sql.SQL(operation), sql.Identifier(table), sql.SQL(deferred)))
    result = workflows.evaluate(run["id"])
    assert result.returncode != 0
    assert execution.snapshot() == before
    assert execution.wakeups() == 1
    connection.execute(sql.SQL("DROP TRIGGER fail ON orchestrator.{}").format(sql.Identifier(table)))
    result = workflows.evaluate(run["id"])
    assert result.returncode == 0, result.stderr
    assert execution.wakeups() == 0


def test_disconnected_evaluator_rolls_back_then_restarts(connection, seed_definition, workflows, database):
    run = workflows.start(seed_definition("sequence.yaml")["id"])
    execution = Execution(connection, run["id"])
    before = execution.snapshot()
    barrier = DatabaseBarrier(connection)
    barrier.pause_wakeup_delete()
    with ThreadPoolExecutor(max_workers=1) as pool:
        with barrier.hold() as holder:
            future = pool.submit(workflows.evaluate, run["id"], DATABASE_URL=make_conninfo(database.dsn, application_name="dying"))
            pid = barrier.wait_blocked("dying", holder)
            assert connection.execute("SELECT pg_terminate_backend(%s)", (pid,)).fetchone() == (True,)
            result = future.result(timeout=8)
            assert result.returncode != 0
    assert execution.snapshot() == before
    assert execution.wakeups() == 1
    result = workflows.evaluate(run["id"])
    assert result.returncode == 0, result.stderr
    assert execution.wakeups() == 0
