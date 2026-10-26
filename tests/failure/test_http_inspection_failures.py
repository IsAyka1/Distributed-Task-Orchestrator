from concurrent.futures import ThreadPoolExecutor

import pytest
from psycopg.conninfo import make_conninfo

from support.evaluation import DatabaseBarrier
from support.http import HTTPClient


def test_task_inspection_keeps_snapshot_across_concurrent_commit(application, connection, clock, running_claim, ready_execution):
    run_id = ready_execution.workflow_id
    dsn = make_conninfo(application.database.dsn, application_name="inspection-snapshot")
    with application.running("-http", binary="probe", DATABASE_URL=dsn, TEST_HTTP_NOW=clock.now.isoformat()) as process:
        client = HTTPClient.from_process(process)
        path = f"/workflows/{run_id}/tasks"
        before = client.request(path, method="GET").body
        with ThreadPoolExecutor(max_workers=1) as pool:
            with connection.transaction():
                ready_execution.lock()
                connection.execute("LOCK TABLE orchestrator.task_runs IN ACCESS EXCLUSIVE MODE")
                ready_execution.complete("A", output={"committed": True})
                pending = pool.submit(client.request, path, method="GET")
                with application.database.connect() as observer:
                    pid = DatabaseBarrier(observer).wait_blocked("inspection-snapshot", connection.info.backend_pid)
                    query = observer.execute("SELECT query FROM pg_stat_activity WHERE pid=%s", (pid,)).fetchone()[0]
                    assert "FROM orchestrator.task_runs" in query
                # The reader has read the run and definition; the task SELECT waits for this commit.
            response = pending.result(timeout=5)
        assert response.status == 200 and response.body == before
        after = client.request(path, method="GET")
        assert after.status == 200
        assert after.body["tasks"][0]["status"] == "SUCCEEDED"
        assert after.body["tasks"][0]["output"] == {"committed": True}
        assert after.body["tasks"][0]["current_attempt_id"] is None
        process.terminate()
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0 and stdout == stderr == ""


@pytest.mark.parametrize("suffix", ["", "/tasks"])
def test_inspection_hides_uncommitted_changes(http_api, connection, ready_execution, suffix):
    path = f"/workflows/{ready_execution.workflow_id}{suffix}"
    before = http_api.request(path, method="GET")
    with connection.transaction(force_rollback=True):
        ready_execution.claim("A")
        ready_execution.complete("A", output={"uncommitted": True})
        response = http_api.request(path, method="GET")
        assert response.status == 200 and response.body == before.body
    assert http_api.request(path, method="GET").body == before.body


@pytest.mark.parametrize("suffix", ["", "/tasks"])
@pytest.mark.parametrize("shutdown", [False, True])
def test_inspection_cancellation_is_sanitized(application, connection, clock, ready_execution, suffix, shutdown):
    dsn = make_conninfo(application.database.dsn, application_name="inspection-cancel")
    with application.running("-http", binary="probe", DATABASE_URL=dsn, TEST_HTTP_NOW=clock.now.isoformat()) as process:
        client = HTTPClient.from_process(process)
        with ThreadPoolExecutor(max_workers=1) as pool:
            with connection.transaction():
                connection.execute("LOCK TABLE orchestrator.workflow_runs IN ACCESS EXCLUSIVE MODE")
                pending = pool.submit(client.request, f"/workflows/{ready_execution.workflow_id}{suffix}", method="GET")
                with application.database.connect() as observer:
                    pid = DatabaseBarrier(observer).wait_blocked("inspection-cancel", connection.info.backend_pid)
                    if shutdown:
                        process.terminate()
                    else:
                        assert observer.execute("SELECT pg_terminate_backend(%s)", (pid,)).fetchone()[0]
                response = pending.result(timeout=5)
                assert response.status == 503 and response.body["error"]["code"] == "storage_unavailable"
                assert "GET requests may be retried" in response.body["error"]["message"]
        if not shutdown:
            process.terminate()
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0 and stdout == stderr == ""


def test_inspection_unexpected_database_error_is_sanitized(http_api, connection, ready_execution):
    connection.execute("ALTER TABLE orchestrator.workflow_runs RENAME COLUMN output TO secret_output")
    response = http_api.request(f"/workflows/{ready_execution.workflow_id}", method="GET")
    assert response.status == 500 and response.body["error"]["code"] == "internal_error"
    assert "secret" not in str(response.body) and "SELECT" not in str(response.body)


@pytest.mark.parametrize("status", ["", "UNKNOWN_SECRET_STATUS"])
@pytest.mark.parametrize("target,suffix", [("workflow", ""), ("workflow", "/tasks"), ("task", "/tasks")])
def test_inspection_unknown_status_is_sanitized(http_api, connection, ready_execution, target, suffix, status):
    run_id = ready_execution.workflow_id
    if target == "workflow":
        connection.execute("UPDATE orchestrator.workflow_runs SET status=%s WHERE id=%s", (status, run_id))
    else:
        connection.execute("UPDATE orchestrator.task_runs SET status=%s WHERE workflow_run_id=%s AND task_key='C'",
                           (status, run_id))
    response = http_api.request(f"/workflows/{run_id}{suffix}", method="GET")
    assert response.status == 500
    assert response.body == {"error": {"code": "internal_error", "message": "The request could not be completed."}}
