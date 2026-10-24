import pytest

from concurrent.futures import ThreadPoolExecutor
from psycopg.conninfo import make_conninfo
from support.evaluation import DatabaseBarrier
from support.http import HTTPClient


def test_shutdown_cancels_http_write_and_worker(application, connection, definition_data):
    dsn = make_conninfo(application.database.dsn, application_name="http-shutdown")
    with application.running("run", DATABASE_URL=dsn, HTTP_ADDR="127.0.0.1:0") as process:
        client = HTTPClient.from_process(process)
        with connection.transaction():
            connection.execute("LOCK TABLE orchestrator.workflow_definitions IN ACCESS EXCLUSIVE MODE")
            with ThreadPoolExecutor(max_workers=1) as pool:
                pending = pool.submit(client.request, "/workflow-definitions", definition_data("sequence.yaml").document())
                with application.database.connect() as observer:
                    DatabaseBarrier(observer).wait_blocked("http-shutdown", connection.info.backend_pid)
                process.terminate()
                response = pending.result(timeout=5)
                assert response.status == 503
                assert response.body["error"]["code"] == "storage_unavailable"
                stdout, stderr = process.communicate(timeout=5)
                assert process.returncode == 0, stderr
                assert stdout == stderr == ""
        assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0


def test_run_serves_publication(application, definition_data):
    application.migrate()
    with application.running("run", HTTP_ADDR="127.0.0.1:0") as process:
        client = HTTPClient.from_process(process)
        result = client.created("/workflow-definitions", definition_data("sequence.yaml").document())
        assert result["id"]
        process.terminate()
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0, stderr
        assert stdout == stderr == ""


def test_database_disconnect_returns_503(application, connection, clock, definition_data):
    dsn = make_conninfo(application.database.dsn, application_name="http-disconnect")
    with application.running("-http", binary="probe", DATABASE_URL=dsn, TEST_HTTP_NOW=clock.now.isoformat()) as process:
        client = HTTPClient.from_process(process)
        with connection.transaction():
            connection.execute("LOCK TABLE orchestrator.workflow_definitions IN ACCESS EXCLUSIVE MODE")
            with ThreadPoolExecutor(max_workers=1) as pool:
                pending = pool.submit(client.request, "/workflow-definitions", definition_data("sequence.yaml").document())
                with application.database.connect() as observer:
                    pid = DatabaseBarrier(observer).wait_blocked("http-disconnect", connection.info.backend_pid)
                    assert observer.execute("SELECT pg_terminate_backend(%s)", (pid,)).fetchone()[0]
                response = pending.result(timeout=5)
                assert response.status == 503
                assert response.body["error"]["code"] == "storage_unavailable"
                assert "may have committed" in response.body["error"]["message"]
        assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0
        process.terminate()
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0, stderr
        assert stdout == stderr == ""


@pytest.mark.parametrize("cancellation", ["deadline", "shutdown"])
def test_cancelled_start_after_automatic_rollback(application, connection, clock, seed_definition, cancellation):
    definition_id = seed_definition("sequence.yaml")["id"]
    with application.running("-http", binary="probe", TEST_HTTP_NOW=clock.now.isoformat(),
                             TEST_HTTP_CANCEL_START=cancellation) as process:
        client = HTTPClient.from_process(process)
        with ThreadPoolExecutor(max_workers=1) as pool:
            pending = pool.submit(client.request, "/workflows", {"definition_id": definition_id})
            if cancellation == "shutdown":
                assert client.read_signal(process) == "start ready to commit"
                process.terminate()
            response = pending.result(timeout=5)
        if cancellation == "deadline":
            process.terminate()
        stdout, stderr = process.communicate(timeout=5)
        assert process.returncode == 0, stderr
        assert stdout == "start automatically rolled back\n"
        assert stderr == ""
        assert response.status == 503, response
        assert response.body["error"]["code"] == "storage_unavailable"
        assert "may have committed" in response.body["error"]["message"]
        for table in ("workflow_runs", "task_runs", "workflow_wakeups"):
            assert connection.execute(f"SELECT count(*) FROM orchestrator.{table}").fetchone()[0] == 0
