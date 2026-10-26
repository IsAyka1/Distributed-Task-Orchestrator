from contextlib import contextmanager
from dataclasses import dataclass

from psycopg import sql


@dataclass
class DatabaseOutage:
    cluster: object
    database: object
    observer_pid: int

    @contextmanager
    def hold(self):
        # Only the disposable database is affected; the observer can verify rollback.
        assert self.database.name.startswith("tm_test_")
        with self.cluster.connect() as admin:
            name = sql.Identifier(self.database.name)
            admin.execute(sql.SQL("ALTER DATABASE {} ALLOW_CONNECTIONS false").format(name))
            try:
                admin.execute("SELECT pg_terminate_backend(pid) FROM pg_stat_activity "
                              "WHERE datname=%s AND pid<>%s", (self.database.name, self.observer_pid))
                yield
            finally:
                admin.execute(sql.SQL("ALTER DATABASE {} ALLOW_CONNECTIONS true").format(name))


def wait_rollback(connection, workflow_id):
    with connection.transaction():
        connection.execute("SET LOCAL statement_timeout = '4s'")
        connection.execute("SELECT id FROM orchestrator.workflow_runs WHERE id=%s FOR UPDATE", (workflow_id,))
