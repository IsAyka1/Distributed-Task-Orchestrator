from contextlib import contextmanager
from dataclasses import dataclass
import time
from uuid import uuid4

from psycopg import sql
from psycopg.types.json import Jsonb


@dataclass
class Execution:
    connection: object
    workflow_id: str

    def signal(self):
        self.connection.execute("INSERT INTO orchestrator.workflow_wakeups (workflow_run_id) VALUES (%s) "
                                "ON CONFLICT DO NOTHING", (self.workflow_id,))

    def lock(self):
        conn = self.connection
        conn.execute("SET LOCAL statement_timeout = '4s'")
        conn.execute("SELECT id FROM orchestrator.workflow_runs WHERE id=%s FOR UPDATE", (self.workflow_id,))
        conn.execute("SELECT workflow_run_id FROM orchestrator.workflow_wakeups WHERE workflow_run_id=%s FOR UPDATE",
                     (self.workflow_id,))
        conn.execute("SELECT id FROM orchestrator.task_runs WHERE workflow_run_id=%s ORDER BY id FOR UPDATE",
                     (self.workflow_id,))
        conn.execute("SELECT a.id FROM orchestrator.task_attempts a JOIN orchestrator.task_runs t ON t.id=a.task_run_id "
                     "WHERE t.workflow_run_id=%s ORDER BY a.id FOR UPDATE OF a", (self.workflow_id,))

    def claim(self, key):
        conn = self.connection
        with conn.transaction():
            self.lock()
            task_id = conn.execute("SELECT id FROM orchestrator.task_runs WHERE workflow_run_id=%s AND task_key=%s",
                                   (self.workflow_id, key)).fetchone()[0]
            attempt_id = uuid4()
            conn.execute("INSERT INTO orchestrator.task_attempts (id, task_run_id, attempt_no, worker_id) "
                         "VALUES (%s, %s, 1, 'fixture')", (attempt_id, task_id))
            conn.execute("UPDATE orchestrator.task_runs SET status='RUNNING', attempt_count=1, current_attempt_id=%s, "
                         "lease_owner='fixture', lease_expires_at=orchestrator.database_now()+interval '1 minute', "
                         "started_at=orchestrator.database_now() WHERE id=%s", (attempt_id, task_id))
            conn.execute("UPDATE orchestrator.workflow_runs SET version=version+1 WHERE id=%s", (self.workflow_id,))

    def complete(self, key, output=None, status="SUCCEEDED"):
        conn = self.connection
        with conn.transaction():
            self.lock()
            task_id, attempt_id = conn.execute(
                "SELECT id, current_attempt_id FROM orchestrator.task_runs WHERE workflow_run_id=%s AND task_key=%s",
                (self.workflow_id, key)).fetchone()
            assert attempt_id is not None
            conn.execute("UPDATE orchestrator.task_attempts SET status=%s, finished_at=orchestrator.database_now() "
                         "WHERE id=%s", (status, attempt_id))
            conn.execute("UPDATE orchestrator.task_runs SET status=%s, output=%s, current_attempt_id=NULL, "
                         "lease_owner=NULL, lease_expires_at=NULL, finished_at=orchestrator.database_now() WHERE id=%s",
                         (status, Jsonb(output), task_id))
            conn.execute("UPDATE orchestrator.workflow_runs SET version=version+1 WHERE id=%s", (self.workflow_id,))
            self.signal()

    def snapshot(self):
        conn = self.connection
        return tuple(conn.execute(query, (self.workflow_id,)).fetchall() for query in [
            "SELECT * FROM orchestrator.workflow_runs WHERE id=%s",
            "SELECT * FROM orchestrator.task_runs WHERE workflow_run_id=%s ORDER BY id",
            "SELECT a.* FROM orchestrator.task_attempts a JOIN orchestrator.task_runs t ON t.id=a.task_run_id "
            "WHERE t.workflow_run_id=%s ORDER BY a.id",
        ])

    def wakeups(self):
        return self.connection.execute("SELECT count(*) FROM orchestrator.workflow_wakeups WHERE workflow_run_id=%s",
                                       (self.workflow_id,)).fetchone()[0]


@dataclass
class DatabaseBarrier:
    connection: object
    key: int = 106

    @contextmanager
    def hold(self):
        self.connection.execute("SELECT pg_advisory_lock(%s)", (self.key,))
        try:
            yield self.connection.info.backend_pid
        finally:
            self.connection.execute("SELECT pg_advisory_unlock(%s)", (self.key,))

    def pause_wakeup_delete(self):
        self.connection.execute(sql.SQL("CREATE FUNCTION public.pause_delete() RETURNS trigger LANGUAGE plpgsql AS {}")
            .format(sql.Literal(f"BEGIN PERFORM pg_advisory_xact_lock({self.key}); RETURN OLD; END;")))
        self.connection.execute("CREATE TRIGGER pause_delete BEFORE DELETE ON orchestrator.workflow_wakeups "
                                "FOR EACH ROW EXECUTE FUNCTION public.pause_delete()")

    def wait_blocked(self, application_name, blocker_pid):
        deadline = time.monotonic() + 3
        # Observe the actual database lock dependency; elapsed time never proves ordering.
        while time.monotonic() < deadline:
            row = self.connection.execute(
                "SELECT pid FROM pg_stat_activity WHERE application_name=%s AND %s=ANY(pg_blocking_pids(pid))",
                (application_name, blocker_pid)).fetchone()
            if row:
                return row[0]
        raise AssertionError(f"{application_name} did not reach its database barrier")
