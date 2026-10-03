import pytest


@pytest.mark.parametrize("stage", ["second_task", "wakeup", "commit"])
def test_start_rolls_back_every_execution_row(seed_definition, workflows, connection, stage):
    published = seed_definition("sequence.yaml")
    # The second task insert proves that an earlier run/task write is undone.
    condition = "NEW.task_key = 'A'" if stage == "second_task" else "true"
    connection.execute(f"""CREATE FUNCTION fail_start() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN
            IF {condition} THEN RAISE EXCEPTION 'injected start failure'; END IF;
            RETURN NEW;
        END $$""")
    if stage == "second_task":
        connection.execute("CREATE TRIGGER fail_start BEFORE INSERT ON orchestrator.task_runs "
                           "FOR EACH ROW EXECUTE FUNCTION fail_start()")
    elif stage == "wakeup":
        connection.execute("CREATE TRIGGER fail_start BEFORE INSERT ON orchestrator.workflow_wakeups "
                           "FOR EACH ROW EXECUTE FUNCTION fail_start()")
    else:
        connection.execute("CREATE CONSTRAINT TRIGGER fail_start AFTER INSERT ON orchestrator.workflow_runs "
                           "DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_start()")
    result = workflows.request(published["id"], input={"n": 1})
    assert result.returncode != 0 and "injected start failure" in result.stderr
    assert not result.stdout.strip()
    assert connection.execute("""SELECT
        (SELECT count(*) FROM orchestrator.workflow_runs),
        (SELECT count(*) FROM orchestrator.task_runs),
        (SELECT count(*) FROM orchestrator.workflow_wakeups),
        (SELECT count(*) FROM orchestrator.task_attempts)""").fetchone() == (0, 0, 0, 0)
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 1
