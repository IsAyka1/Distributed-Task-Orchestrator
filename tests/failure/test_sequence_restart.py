import signal

from psycopg.conninfo import make_conninfo
import pytest

from support.evaluation import DatabaseBarrier, Execution
from support.outages import DatabaseOutage, wait_rollback
from support.workers import Workers


@pytest.fixture
def workers(application, clock):
    return Workers(application, clock)


@pytest.fixture
def committed_a(connection, seed_definition, workflows, workers):
    run = workflows.start(seed_definition("sequence.yaml")["id"], input={"n": 1})
    result = workers.step(count=1)
    assert [call["Key"] for call in result["Calls"]] == ["A"]
    assert result["Worked"] == [True]
    execution = Execution(connection, run["id"])
    assert connection.execute("SELECT task_key, status, attempt_count FROM orchestrator.task_runs "
                              "WHERE workflow_run_id=%s ORDER BY task_key", (run["id"],)).fetchall() == [
        ("A", "SUCCEEDED", 1), ("B", "PENDING", 0), ("C", "PENDING", 0)]
    assert execution.wakeups() == 1
    return execution


def assert_resumed(execution, workers, client):
    conn, run_id = execution.connection, execution.workflow_id
    a_before = conn.execute("SELECT * FROM orchestrator.task_runs WHERE workflow_run_id=%s AND task_key='A'",
                            (run_id,)).fetchone()
    attempts_before = conn.execute("SELECT a.* FROM orchestrator.task_attempts a JOIN orchestrator.task_runs t "
                                   "ON t.id=a.task_run_id WHERE t.workflow_run_id=%s AND t.task_key='A'",
                                   (run_id,)).fetchall()
    result = workers.step(count=4)
    assert [call["Key"] for call in result["Calls"]] == ["B", "C"]
    assert [call["Input"]["Value"]["n"] for call in result["Calls"]] == [2, 3]
    assert result["Worked"] == [True, True, True, False]
    assert conn.execute("SELECT * FROM orchestrator.task_runs WHERE workflow_run_id=%s AND task_key='A'",
                        (run_id,)).fetchone() == a_before
    assert conn.execute("SELECT a.* FROM orchestrator.task_attempts a JOIN orchestrator.task_runs t "
                        "ON t.id=a.task_run_id WHERE t.workflow_run_id=%s AND t.task_key='A'",
                        (run_id,)).fetchall() == attempts_before
    response = client.request(f"/workflows/{run_id}/tasks", method="GET")
    assert response.status == 200
    tasks = response.body["tasks"]
    assert [task["task_key"] for task in tasks] == ["A", "B", "C"]
    assert all(task["status"] == "SUCCEEDED" and task["attempt_count"] == 1
               and task["current_attempt_id"] is None for task in tasks)
    task_ids = [task["id"] for task in tasks]
    assert [call["TaskID"] for call in result["Calls"]] == task_ids[1:]
    response = client.request(f"/workflows/{run_id}", method="GET")
    assert response.status == 200
    assert response.body["status"] == "SUCCEEDED"
    assert response.body["output"] == {"n": 4, "keys": task_ids}
    assert response.body["finished_at"] is not None
    assert conn.execute("SELECT a.status, count(*) FROM orchestrator.task_attempts a JOIN orchestrator.task_runs t "
                        "ON t.id=a.task_run_id WHERE t.workflow_run_id=%s GROUP BY a.status",
                        (run_id,)).fetchall() == [("SUCCEEDED", 3)]
    assert execution.wakeups() == 0
    before = execution.snapshot()
    assert workers.step(count=1)["Calls"] == []
    assert execution.snapshot() == before


def test_restart_between_attempts_preserves_a(committed_a, workers, http_api):
    assert_resumed(committed_a, workers, http_api)


def test_killed_engine_preserves_wakeup_and_committed_progress(committed_a, workers, http_api, database):
    execution = committed_a
    before = execution.snapshot()
    barrier = DatabaseBarrier(execution.connection)
    barrier.pause_wakeup_delete()
    with barrier.hold() as holder:
        dsn = make_conninfo(database.dsn, application_name="sequence-engine-crash")
        with workers.running(DATABASE_URL=dsn) as process:
            barrier.wait_blocked("sequence-engine-crash", holder)
            process.kill()
            stdout, _ = process.communicate(timeout=5)
            assert process.returncode == -signal.SIGKILL and stdout == ""
        wait_rollback(execution.connection, execution.workflow_id)
        assert execution.snapshot() == before
        assert execution.wakeups() == 1
    assert_resumed(execution, workers, http_api)


def test_failed_engine_commit_is_not_reported_as_success(committed_a, workers, http_api):
    execution = committed_a
    conn = execution.connection
    before = execution.snapshot()
    conn.execute("CREATE FUNCTION public.fail_commit() RETURNS trigger LANGUAGE plpgsql AS "
                 "'BEGIN RAISE EXCEPTION ''injected evaluation commit failure''; END;'")
    conn.execute("CREATE CONSTRAINT TRIGGER fail_commit AFTER DELETE ON orchestrator.workflow_wakeups "
                 "DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.fail_commit()")
    try:
        result = workers.request()
        assert result.returncode != 0 and result.stdout == "" and result.stderr
        assert execution.snapshot() == before
        assert execution.wakeups() == 1
    finally:
        conn.execute("DROP TRIGGER fail_commit ON orchestrator.workflow_wakeups")
        conn.execute("DROP FUNCTION public.fail_commit()")
    assert_resumed(execution, workers, http_api)


def test_database_outage_rolls_back_then_restart_resumes(committed_a, workers, http_api, database, cluster):
    execution = committed_a
    before = execution.snapshot()
    conn = execution.connection
    barrier = DatabaseBarrier(conn)
    barrier.pause_wakeup_delete()
    with barrier.hold() as holder:
        dsn = make_conninfo(database.dsn, application_name="sequence-outage")
        with workers.running(DATABASE_URL=dsn) as process:
            barrier.wait_blocked("sequence-outage", holder)
            with DatabaseOutage(cluster, database, conn.info.backend_pid).hold():
                stdout, stderr = process.communicate(timeout=5)
                assert process.returncode != 0 and stdout == "" and stderr
                wait_rollback(conn, execution.workflow_id)
                assert execution.snapshot() == before
                assert execution.wakeups() == 1
                failed_start = workers.request()
                assert failed_start.returncode != 0 and failed_start.stdout == ""
                response = http_api.request(f"/workflows/{execution.workflow_id}", method="GET")
                assert response.status == 503 and response.body["error"]["code"] == "storage_unavailable"
    assert_resumed(execution, workers, http_api)
