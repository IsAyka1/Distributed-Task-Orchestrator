from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import threading

from conftest import Application


def test_setup_cleanup_twice(database_factory, cluster):
    with database_factory() as survivor:
        for _ in range(2):
            with database_factory() as database:
                app = Application(database)
                app.migrate()
                app.migrate()
                assert app.run("db-check").returncode == 0
                with database.connect() as conn:
                    assert conn.execute("SELECT max(version_id) FROM goose_db_version").fetchone() == (1,)
            with cluster.connect() as admin:
                assert admin.execute("SELECT 1 FROM pg_database WHERE datname=%s", (database.name,)).fetchone() is None
            with survivor.connect() as conn:
                assert conn.execute("SELECT 1").fetchone() == (1,)


def test_failed_migration_rolls_back_and_retries(application, migration_dir):
    first = "CREATE TABLE item (id integer PRIMARY KEY);"
    directory = migration_dir(first, "INSERT INTO item VALUES (1); SELECT missing_column;")
    result = application.run("-migrations", directory, binary="probe")
    assert result.returncode != 0
    with application.database.connect() as conn:
        assert conn.execute("SELECT count(*) FROM item").fetchone() == (0,)
        assert conn.execute("SELECT max(version_id) FROM goose_db_version").fetchone() == (1,)
    directory = migration_dir(first, "INSERT INTO item VALUES (1);")
    for _ in range(2):
        result = application.run("-migrations", directory, binary="probe")
        assert result.returncode == 0, result.stderr
    with application.database.connect() as conn:
        assert conn.execute("SELECT id FROM item").fetchall() == [(1,)]
        assert conn.execute("SELECT max(version_id) FROM goose_db_version").fetchone() == (2,)


def test_goose_rejects_duplicate_versions(application, migration_dir):
    directory = migration_dir("SELECT 1;")
    (Path(directory) / "0001_duplicate.sql").write_text("-- +goose Up\nSELECT 2;")
    result = application.run("-migrations", directory, binary="probe")
    assert result.returncode != 0


def test_concurrent_migrations(application):
    barrier = threading.Barrier(2, timeout=5)
    def migrate():
        barrier.wait()
        return application.run("migrate")
    with ThreadPoolExecutor(max_workers=2) as pool:
        futures = [pool.submit(migrate) for _ in range(2)]
        for future in futures:
            result = future.result(timeout=10)
            assert result.returncode == 0, result.stderr
    with application.database.connect() as conn:
        assert conn.execute("SELECT count(*) FROM goose_db_version WHERE version_id=1").fetchone() == (1,)
