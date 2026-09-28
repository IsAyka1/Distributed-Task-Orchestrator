import secrets
import socket
from urllib.parse import quote

from psycopg.conninfo import make_conninfo
import pytest


@pytest.mark.parametrize("case", ["missing", "malformed", "authentication"])
def test_invalid_configuration_is_actionable_and_redacted(application, case):
    token = secrets.token_urlsafe(32)
    if case == "missing":
        dsn, expected = "", "DATABASE_URL is required"
    elif case == "malformed":
        dsn, expected = f"postgres://test:{quote(token)}%ZZ@host/db", "invalid DATABASE_URL"
    else:
        dsn = make_conninfo(application.database.dsn, password=token)
        expected = "connect to PostgreSQL"
    result = application.run("db-check", DATABASE_URL=dsn)
    assert result.returncode != 0
    assert expected in result.stderr
    if token in result.stderr:
        pytest.fail("driver error disclosed input")


def test_unreachable_database_fails(application):
    with socket.socket() as reserved:
        reserved.bind(("127.0.0.1", 0))
        dsn = make_conninfo(application.database.dsn, host="127.0.0.1", port=reserved.getsockname()[1])
        result = application.run("db-check", DATABASE_URL=dsn, DATABASE_TIMEOUT="200ms")
    assert result.returncode != 0
    assert "connect to PostgreSQL" in result.stderr


def test_pending_migration_lock_timeout_releases_resources(application):
    with application.database.connect() as blocker:
        blocker.execute("SELECT pg_advisory_lock(4097083626)")
        result = application.run("migrate", DATABASE_TIMEOUT="200ms")
        assert result.returncode != 0
        assert "DATABASE_TIMEOUT" in result.stderr
    application.migrate()


@pytest.mark.parametrize("case", ["connection", "startup", "process"])
def test_fixture_failures_do_not_disclose_runtime_configuration(application, tmp_path, case):
    import os
    import subprocess
    import sys

    token = secrets.token_urlsafe(32)
    dsn = make_conninfo(application.database.dsn, password=token)
    scenario = tmp_path / "test_fixture_error.py"
    scenarios = {
        "connection": "    Database('invalid-auth', os.environ['PROBE_DATABASE_URL']).connect()\n",
        "startup": "    conftest.time = SimpleNamespace(monotonic=iter([0, 31]).__next__)\n"
                   "    conftest.cluster.__wrapped__()\n",
        "process": "    def fail(*args, **kwargs):\n"
                   "        raise subprocess.TimeoutExpired(args[0], 1)\n"
                   "    conftest.subprocess = SimpleNamespace(run=fail, SubprocessError=subprocess.SubprocessError)\n"
                   "    Application(Database('timeout', os.environ['PROBE_DATABASE_URL'])).run(\n"
                   "        'db-check', DATABASE_URL=os.environ['PROBE_DATABASE_URL'])\n",
    }
    scenario.write_text(
        "import os, subprocess, conftest\nfrom types import SimpleNamespace\n"
        "from conftest import Application, Database\ndef test_failure():\n" + scenarios[case]
    )
    try:
        result = subprocess.run(
            [sys.executable, "-m", "pytest", "-p", "no:cacheprovider", "-q", str(scenario)],
            env=dict(os.environ, PYTHONPATH="/workspace/tests", PROBE_DATABASE_URL=dsn,
                     TEST_DATABASE_PASSWORD=token),
            capture_output=True, text=True, timeout=15,
        )
    except (OSError, subprocess.SubprocessError):
        pytest.fail("nested failure scenario could not complete", pytrace=False)
    if result.returncode != 1:
        pytest.fail("nested fixture failure scenario did not fail as expected")
    if token in result.stdout or token in result.stderr:
        pytest.fail("fixture failure disclosed runtime configuration")
    expected = {
        "connection": "test database connection failed; check harness availability",
        "startup": "test PostgreSQL did not become ready",
        "process": "Go test process failed or timed out",
    }[case]
    if expected not in result.stdout:
        pytest.fail("nested scenario failed for an unexpected reason")
