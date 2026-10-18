from psycopg.conninfo import make_conninfo

from support.evaluation import DatabaseBarrier


def test_sigterm_cancels_worker_waiting_for_database(application, connection):
    # Observe a lock wait before signalling; wall time only bounds the harness.
    with connection.transaction():
        connection.execute("LOCK TABLE orchestrator.workflow_wakeups IN ACCESS EXCLUSIVE MODE")
        dsn = make_conninfo(application.database.dsn, application_name="worker-shutdown")
        with application.running("run", DATABASE_URL=dsn) as process:
            # Observe from autocommit so pg_stat_activity does not retain a transaction snapshot.
            with application.database.connect() as observer:
                DatabaseBarrier(observer).wait_blocked("worker-shutdown", connection.info.backend_pid)
            process.terminate()
            stdout, stderr = process.communicate(timeout=5)
            assert process.returncode == 0, stderr
            assert stdout == stderr == ""
