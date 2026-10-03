from contextlib import contextmanager
from pathlib import Path
from support.definitions import DefinitionData, Definitions
from support.workflows import Workflows
from dataclasses import dataclass, field
from datetime import datetime, timezone, timedelta
import os
import json
import subprocess
import time
import uuid

import psycopg
from psycopg import sql
from psycopg.conninfo import make_conninfo
import pytest

INITIAL_TIME = datetime(2030, 1, 2, 3, 4, 5, tzinfo=timezone.utc)


@dataclass(frozen=True)
class Database:
    name: str
    dsn: str = field(repr=False)

    def connect(self):
        try:
            return psycopg.connect(self.dsn, autocommit=True, connect_timeout=3)
        except psycopg.Error:
            raise RuntimeError("test database connection failed; check harness availability") from None


@dataclass
class Application:
    database: Database

    def run(self, *args, binary="orchestrator", input=None, **environment):
        env = {"PATH": os.environ["PATH"], "DATABASE_URL": self.database.dsn, "DATABASE_TIMEOUT": "5s"}
        env.update(environment)
        try:
            return subprocess.run([f"/binaries/{binary}", *args], env=env,
                                  text=True, input=input, capture_output=True, timeout=10)
        except (OSError, subprocess.SubprocessError):
            pytest.fail("Go test process failed or timed out", pytrace=False)

    def migrate(self):
        result = self.run("migrate")
        assert result.returncode == 0, result.stderr


@dataclass(frozen=True)
class ClockSnapshot:
    application: datetime
    database: datetime


@dataclass
class Clock:
    database: Database
    now: datetime = INITIAL_TIME

    def set(self, instant):
        with self.database.connect() as conn:
            body = sql.SQL("SELECT {}::timestamptz").format(sql.Literal(instant.isoformat())).as_string(conn)
            conn.execute(sql.SQL("CREATE OR REPLACE FUNCTION orchestrator.database_now() RETURNS timestamptz "
                                 "LANGUAGE sql VOLATILE AS {}").format(sql.Literal(body)))
        self.now = instant

    def advance(self, duration: timedelta):
        self.set(self.now + duration)

    def snapshot(self, application):
        result = application.run("-now", self.now.isoformat(), binary="probe")
        assert result.returncode == 0, result.stderr
        values = json.loads(result.stdout)
        return ClockSnapshot(datetime.fromisoformat(values["application"]),
                             datetime.fromisoformat(values["database"]))


@pytest.fixture(scope="session")
def cluster():
    credential = os.environ["TEST_DATABASE_PASSWORD"]
    if not credential:
        pytest.fail("test database runtime configuration is missing")
    dsn = make_conninfo(host="database", user="postgres", dbname="postgres",
                        password=credential, sslmode="disable")
    deadline = time.monotonic() + 30
    while True:
        try:
            with psycopg.connect(dsn, autocommit=True, connect_timeout=2) as conn:
                conn.execute("SELECT 1")
            break
        except psycopg.OperationalError:
            if time.monotonic() >= deadline:
                pytest.fail("test PostgreSQL did not become ready", pytrace=False)
            time.sleep(0.1)  # Infrastructure readiness only; never advances business time.
    return Database("postgres", dsn)


@pytest.fixture
def database_factory(cluster):
    @contextmanager
    def create():
        name = "tm_test_" + uuid.uuid4().hex
        with cluster.connect() as admin:
            admin.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
        try:
            yield Database(name, make_conninfo(cluster.dsn, dbname=name))
        finally:
            with cluster.connect() as admin:
                admin.execute(sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name)))
    return create


@pytest.fixture
def database(database_factory):
    with database_factory() as value:
        yield value


@pytest.fixture
def application(database):
    return Application(database)


@pytest.fixture
def clock(application):
    application.migrate()
    value = Clock(application.database)
    value.set(INITIAL_TIME)
    yield value
    # The enclosing database fixture drops this entire test-only database.


@pytest.fixture
def migration_dir(tmp_path):
    def write(*statements):
        for path in tmp_path.glob("*.sql"):
            path.unlink()
        for i, statement in enumerate(statements, 1):
            (tmp_path / f"{i:04d}_test.sql").write_text("-- +goose Up\n" + statement)
        return str(tmp_path)
    return write


@pytest.fixture
def connection(application, clock):
    with application.database.connect() as conn:
        yield conn


@pytest.fixture
def definition_data():
    def load(filename="dag.yaml"):
        return DefinitionData.from_yaml(Path(__file__).parent / "fixtures" / "definitions" / filename)
    return load


@pytest.fixture
def definitions(application, clock):
    return Definitions(application)


@pytest.fixture
def seed_definition(definitions, definition_data):
    def seed(filename="dag.yaml"):
        return definitions.succeed("publish", definition_data(filename).document())
    return seed


@pytest.fixture
def workflows(application, clock):
    return Workflows(application)
