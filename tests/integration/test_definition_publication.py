from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from dataclasses import dataclass
from datetime import timedelta
import json
from threading import Barrier
from uuid import UUID

import pytest


@dataclass
class Definitions:
    application: object

    def request(self, action, document):
        return self.application.run("-definition", action, binary="probe", input=json.dumps(document))

    def succeed(self, action, document):
        result = self.request(action, document)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)


@pytest.fixture
def definitions(application, clock):
    return Definitions(application)


def definition():
    # Declaration order is deliberately different from graph order.
    return {"provider": " Demo ", "name": " Flow ", "version": 1, "tasks": [
        {"id": "end", "type": "activity", "depends_on": ["left", "right"], "max_attempts": 3},
        {"id": "right", "type": "activity", "depends_on": [], "max_attempts": 1},
        {"id": "left", "type": "activity", "depends_on": [], "max_attempts": 2147483647},
    ]}


def test_roundtrip_new_version_and_prior_version_unchanged(definitions, clock):
    original = definition()
    first = definitions.succeed("publish", original)
    assert UUID(first["id"]).version == 4
    assert first["definition"] == original
    assert first["created_at"] == clock.now.isoformat().replace("+00:00", "Z")
    clock.advance(timedelta(days=1))
    newer = deepcopy(original)
    newer["version"] = 9
    newer["tasks"][0]["max_attempts"] = 7
    second = definitions.succeed("publish", newer)
    assert second["id"] != first["id"]
    assert second["definition"] == newer
    assert second["created_at"] != first["created_at"]
    assert definitions.succeed("get", original) == first
    assert definitions.succeed("get", newer) == second
    for duplicate in (original, dict(original, tasks=newer["tasks"])):
        result = definitions.request("publish", duplicate)
        assert result.returncode != 0
        assert "definition_version_conflict" in result.stderr
    assert definitions.succeed("get", original) == first


def test_defaults_and_exact_identity(definitions):
    original = definition()
    del original["tasks"][0]["max_attempts"]
    result = definitions.succeed("publish", original)
    assert result["definition"]["tasks"][0]["max_attempts"] == 1
    assert definitions.succeed("get", original) == result
    for field, value in (("provider", " Demo"), ("provider", " demo "), ("name", " flow ")):
        other = dict(original, **{field: value})
        missing = definitions.request("get", other)
        assert missing.returncode != 0 and "definition_not_found" in missing.stderr
        published = definitions.succeed("publish", other)
        assert published["id"] != result["id"]
        assert definitions.succeed("get", other) == published


def test_concurrent_same_version_has_one_winner(definitions, connection):
    barrier = Barrier(2, timeout=5)
    documents = [definition(), definition()]
    documents[1]["tasks"][0]["max_attempts"] = 5

    def publish(document):
        barrier.wait()
        return definitions.request("publish", document)

    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(publish, documents))
    winners = [json.loads(r.stdout) for r in results if r.returncode == 0]
    losers = [r for r in results if r.returncode != 0]
    assert len(winners) == len(losers) == 1
    assert "definition_version_conflict" in losers[0].stderr
    assert definitions.succeed("get", documents[0]) == winners[0]
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 1


@pytest.mark.parametrize("invalid", [
    {"provider": ""}, {"name": ""}, {"name": "nul\x00name"}, {"version": 0}, {"tasks": []},
    {"tasks": [{"id": "", "type": "activity"}]},
    {"tasks": [{"id": "a", "type": "timer"}]},
    {"tasks": [{"id": "a", "type": "activity", "depends_on": ["missing"]}]},
    {"tasks": [{"id": "a", "type": "activity", "depends_on": ["a"]}]},
    {"tasks": [{"id": "a", "type": "activity", "max_attempts": 0}]},
    {"tasks": [{"id": "a", "type": "activity", "max_attempts": 2147483648}]},
    {"tasks": [{"id": "a", "type": "activity"}, {"id": "a", "type": "activity"}]},
])
def test_invalid_publication_writes_nothing(definitions, connection, invalid):
    result = definitions.request("publish", dict(definition(), **invalid))
    assert result.returncode != 0
    assert "invalid_definition" in result.stderr
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0


def test_read_rejects_corrupt_stored_content(definitions, connection):
    connection.execute("""INSERT INTO orchestrator.workflow_definitions
        (id, provider, name, version, definition)
        VALUES (gen_random_uuid(), ' Demo ', ' Flow ', 1, '{"tasks": []}')""")
    result = definitions.request("get", definition())
    assert result.returncode != 0 and "invalid stored definition" in result.stderr
