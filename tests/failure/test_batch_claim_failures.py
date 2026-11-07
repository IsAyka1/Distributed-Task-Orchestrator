from psycopg import sql
import pytest


@pytest.mark.parametrize("stage", ["second_attempt", "commit"])
def test_batch_failure_returns_nothing_and_rolls_back_all_workflows(ready_batch, claims, stage):
    conn = ready_batch[0].connection
    before = [execution.snapshot() for execution in ready_batch]
    conn.execute("CREATE FUNCTION public.fail_batch() RETURNS trigger LANGUAGE plpgsql AS $$ "
                 "BEGIN IF (SELECT count(*) FROM orchestrator.task_attempts) >= 2 THEN "
                 "RAISE EXCEPTION 'injected batch failure'; END IF; RETURN NEW; END; $$")
    trigger = ("CONSTRAINT TRIGGER fail_batch AFTER INSERT ON orchestrator.task_attempts DEFERRABLE INITIALLY DEFERRED"
               if stage == "commit" else "TRIGGER fail_batch AFTER INSERT ON orchestrator.task_attempts")
    conn.execute(sql.SQL("CREATE {} FOR EACH ROW EXECUTE FUNCTION public.fail_batch()").format(sql.SQL(trigger)))
    result = claims.request(limit=4)
    assert result.returncode != 0
    assert "injected batch failure" in result.stderr
    assert result.stdout == ""
    assert [execution.snapshot() for execution in ready_batch] == before
    assert all(execution.wakeups() == 0 for execution in ready_batch)
    conn.execute("DROP TRIGGER fail_batch ON orchestrator.task_attempts")
    claimed = claims.claim(limit=4)
    assert len(claimed) == 4
    assert all(c["Attempt"]["Number"] == 1 for c in claimed)
