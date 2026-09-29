from dataclasses import dataclass
from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta
from pathlib import Path
import shutil
import threading
from uuid import UUID, uuid4

import psycopg
from psycopg import sql
import pytest

from conftest import Clock, INITIAL_TIME
from support.schema import Definition, insert_run


@dataclass
class TaskRows:
    conn: psycopg.Connection
    workflow_id: UUID

    def task(self, key="A", *, limit=1, count=0, status="PENDING"):
        identifier = uuid4()
        self.conn.execute(
            "INSERT INTO orchestrator.task_runs "
            "(id, workflow_run_id, task_key, max_attempt_count, attempt_count, status) "
            "VALUES (%s, %s, %s, %s, %s, %s)",
            (identifier, self.workflow_id, key, limit, count, status),
        )
        return identifier

    def attempt(self, task_id, *, number=1, status="RUNNING", worker="worker"):
        identifier = uuid4()
        self.conn.execute(
            "INSERT INTO orchestrator.task_attempts (id, task_run_id, attempt_no, status, worker_id) "
            "VALUES (%s, %s, %s, %s, %s)", (identifier, task_id, number, status, worker),
        )
        return identifier

    def claim(self, task_id, attempt_id):
        self.conn.execute(
            "UPDATE orchestrator.task_runs SET status='RUNNING', attempt_count=1, "
            "current_attempt_id=%s, lease_owner='worker', lease_expires_at=%s WHERE id=%s",
            (attempt_id, INITIAL_TIME + timedelta(minutes=1), task_id),
        )


@pytest.fixture
def rows(connection):
    return TaskRows(connection, insert_run(connection, Definition().insert(connection)))


def test_task_identity_defaults_and_workflow_fk(rows):
    task_id = rows.task()
    assert rows.conn.execute(
        "SELECT status, max_attempt_count, attempt_count, input, output, "
        "input IS NULL, output IS NULL, current_attempt_id, lease_owner, lease_expires_at "
        "FROM orchestrator.task_runs WHERE id=%s", (task_id,),
    ).fetchone() == ("PENDING", 1, 0, None, None, False, False, None, None, None)
    with pytest.raises(psycopg.errors.UniqueViolation):
        rows.task()
    rows.task("a")
    rows.task("A ")
    another = TaskRows(rows.conn, insert_run(rows.conn, Definition(version=2).insert(rows.conn)))
    another.task()
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        TaskRows(rows.conn, uuid4()).task()


@pytest.mark.parametrize("limit,count", [(0, 0), (-1, 0), (2, 0), (1, -1), (1, 2)])
def test_single_attempt_budget(rows, limit, count):
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.task(limit=limit, count=count)


@pytest.mark.parametrize("key,status", [("", "PENDING"), ("A", "RETRY_WAIT"), ("A", "unknown")])
def test_task_key_and_status(rows, key, status):
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.task(key, status=status)


def test_attempt_identity_and_parent(rows):
    task_id = rows.task()
    rows.attempt(task_id)
    with pytest.raises(psycopg.errors.UniqueViolation):
        rows.attempt(task_id)
    rows.attempt(rows.task("B"))
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        rows.attempt(uuid4())


@pytest.mark.parametrize("number,status,worker", [
    (0, "RUNNING", "worker"), (-1, "RUNNING", "worker"), (2, "RUNNING", "worker"),
    (1, "PENDING", "worker"), (1, "LOST_LEASE", "worker"), (1, "RUNNING", ""),
])
def test_attempt_validation(rows, number, status, worker):
    task_id = rows.task()
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.attempt(task_id, number=number, status=status, worker=worker)


def test_active_attempt_must_belong_to_task(rows):
    first, second = rows.task(), rows.task("B")
    attempt = rows.attempt(first)
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        rows.claim(second, attempt)
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        rows.claim(first, uuid4())
    rows.claim(first, attempt)
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        rows.conn.execute("DELETE FROM orchestrator.task_attempts WHERE id=%s", (attempt,))


@pytest.mark.parametrize("column", ["current_attempt_id", "lease_owner", "lease_expires_at"])
def test_running_requires_all_lease_fields(rows, column):
    task_id = rows.task()
    rows.claim(task_id, rows.attempt(task_id))
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.conn.execute(sql.SQL("UPDATE orchestrator.task_runs SET {}=NULL WHERE id=%s")
                          .format(sql.Identifier(column)), (task_id,))


@pytest.mark.parametrize("assignment", ["lease_owner=''", "attempt_count=0"])
def test_running_requires_owner_and_consumed_attempt(rows, assignment):
    task_id = rows.task()
    rows.claim(task_id, rows.attempt(task_id))
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.conn.execute(sql.SQL("UPDATE orchestrator.task_runs SET {} WHERE id=%s")
                          .format(sql.SQL(assignment)), (task_id,))


@pytest.mark.parametrize("column", ["max_attempt_count", "attempt_count", "status", "input", "output"])
def test_required_task_values(rows, column):
    task_id = rows.task()
    with pytest.raises(psycopg.errors.NotNullViolation):
        rows.conn.execute(sql.SQL("UPDATE orchestrator.task_runs SET {}=NULL WHERE id=%s")
                          .format(sql.Identifier(column)), (task_id,))


@pytest.mark.parametrize("status", ["PENDING", "READY", "SUCCEEDED", "FAILED"])
def test_nonrunning_requires_clearing_all_lease_fields(rows, status):
    task_id = rows.task()
    attempt = rows.attempt(task_id)
    rows.claim(task_id, attempt)
    with pytest.raises(psycopg.errors.CheckViolation):
        rows.conn.execute("UPDATE orchestrator.task_runs SET status=%s WHERE id=%s", (status, task_id))
    rows.conn.execute(
        "UPDATE orchestrator.task_runs SET status=%s, current_attempt_id=NULL, "
        "lease_owner=NULL, lease_expires_at=NULL WHERE id=%s", (status, task_id),
    )


def test_task_and_wakeup_transaction_rolls_back(rows):
    task_id = rows.task()
    with pytest.raises(psycopg.errors.CheckViolation):
        with rows.conn.transaction():
            rows.conn.execute("UPDATE orchestrator.task_runs SET status='READY' WHERE id=%s", (task_id,))
            rows.conn.execute("INSERT INTO orchestrator.workflow_wakeups (workflow_run_id) VALUES (%s)",
                              (rows.workflow_id,))
            rows.conn.execute("UPDATE orchestrator.task_runs SET attempt_count=-1 WHERE id=%s", (task_id,))
    assert rows.conn.execute("SELECT status FROM orchestrator.task_runs WHERE id=%s", (task_id,)).fetchone() == ("PENDING",)
    assert rows.conn.execute("SELECT count(*) FROM orchestrator.workflow_wakeups").fetchone() == (0,)


def test_claim_rollback_restores_counter_and_attempt_history(rows):
    task_id = rows.task(status="READY")
    with pytest.raises(RuntimeError, match="rollback"):
        with rows.conn.transaction():
            rows.claim(task_id, rows.attempt(task_id))
            raise RuntimeError("rollback")
    assert rows.conn.execute(
        "SELECT status, attempt_count, current_attempt_id FROM orchestrator.task_runs WHERE id=%s", (task_id,),
    ).fetchone() == ("READY", 0, None)
    assert rows.conn.execute("SELECT count(*) FROM orchestrator.task_attempts").fetchone() == (0,)


@pytest.mark.parametrize("status", ["SUCCEEDED", "FAILED"])
def test_attempt_history_survives_completion(rows, status):
    task_id = rows.task()
    attempt_id = rows.attempt(task_id)
    rows.claim(task_id, attempt_id)
    with rows.conn.transaction():
        rows.conn.execute(
            "UPDATE orchestrator.task_runs SET status=%s, current_attempt_id=NULL, "
            "lease_owner=NULL, lease_expires_at=NULL WHERE id=%s", (status, task_id),
        )
        rows.conn.execute("UPDATE orchestrator.task_attempts SET status=%s, finished_at=%s WHERE id=%s",
                          (status, INITIAL_TIME, attempt_id))
    assert rows.conn.execute(
        "SELECT task_run_id, attempt_no, status FROM orchestrator.task_attempts WHERE id=%s", (attempt_id,),
    ).fetchone() == (task_id, 1, status)


def test_wakeup_deduplication_and_parent(rows):
    for _ in range(2):
        rows.conn.execute("INSERT INTO orchestrator.workflow_wakeups (workflow_run_id) VALUES (%s) "
                          "ON CONFLICT DO NOTHING", (rows.workflow_id,))
    assert rows.conn.execute("SELECT workflow_run_id FROM orchestrator.workflow_wakeups").fetchall() == [(rows.workflow_id,)]
    with pytest.raises(psycopg.errors.ForeignKeyViolation):
        rows.conn.execute("INSERT INTO orchestrator.workflow_wakeups (workflow_run_id) VALUES (%s)", (uuid4(),))


def test_concurrent_wakeups_are_deduplicated(rows, application):
    barrier = threading.Barrier(2, timeout=5)

    def signal():
        with application.database.connect() as conn:
            conn.execute("SET statement_timeout = '5s'")
            barrier.wait()
            conn.execute("INSERT INTO orchestrator.workflow_wakeups (workflow_run_id) VALUES (%s) "
                         "ON CONFLICT DO NOTHING", (rows.workflow_id,))

    with ThreadPoolExecutor(max_workers=2) as pool:
        futures = [pool.submit(signal) for _ in range(2)]
        for future in futures:
            future.result(timeout=10)
    assert rows.conn.execute("SELECT count(*) FROM orchestrator.workflow_wakeups").fetchone() == (1,)


@pytest.mark.parametrize("name,columns,status", [
    ("task_runs_ready", ["available_at", "id"], "READY"),
    ("task_runs_running", ["lease_expires_at"], "RUNNING"),
])
def test_queue_index_definitions(rows, name, columns, status):
    index = "orchestrator." + name
    assert rows.conn.execute(
        "SELECT pg_get_indexdef(%s::regclass, n, true) FROM generate_series(1, %s) n ORDER BY n",
        (index, len(columns)),
    ).fetchall() == [(column,) for column in columns]
    predicate = rows.conn.execute(
        "SELECT pg_get_expr(indpred, indrelid) FROM pg_index WHERE indexrelid=%s::regclass", (index,),
    ).fetchone()[0]
    assert predicate == f"(status = '{status}'::text)"


def test_migration_over_definition_schema(application, tmp_path):
    for name in ["0001_database_time.sql", "0002_workflow_definitions_runs.sql"]:
        shutil.copyfile(Path("/workspace/migrations") / name, tmp_path / name)
    result = application.run("-migrations", str(tmp_path), binary="probe")
    assert result.returncode == 0, result.stderr
    Clock(application.database).set(INITIAL_TIME)
    with application.database.connect() as conn:
        workflow_id = insert_run(conn, Definition().insert(conn))
        assert conn.execute("SELECT to_regclass('orchestrator.task_runs')").fetchone() == (None,)
        application.migrate()
        application.migrate()
        rows = TaskRows(conn, workflow_id)
        task_id = rows.task()
        rows.claim(task_id, rows.attempt(task_id))
        assert conn.execute("SELECT count(*) FROM orchestrator.workflow_runs").fetchone() == (1,)
