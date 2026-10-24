from concurrent.futures import ThreadPoolExecutor
from threading import Barrier
from uuid import UUID
import json

import pytest


def test_publish_start_and_repeat_are_durable(http_api, connection, definition_data):
    document = definition_data("sequence.yaml").document()
    definition = http_api.created("/workflow-definitions", document)
    assert UUID(definition["id"]).version == 4
    assert definition == {"id": definition["id"], "provider": document["provider"], "name": document["name"], "version": 1}
    assert connection.execute("SELECT name, version, provider FROM orchestrator.workflow_definitions WHERE id=%s", (definition["id"],)).fetchone() == (document["name"], 1, document["provider"])
    runs = [http_api.created("/workflows", {"definition_id": definition["id"], "input": {"n": 1}}) for _ in range(2)]
    assert runs[0]["id"] != runs[1]["id"]
    for run in runs:
        assert run["status"] == "PENDING"
        assert connection.execute("SELECT status, input, version FROM orchestrator.workflow_runs WHERE id=%s", (run["id"],)).fetchone() == ("PENDING", {"n": 1}, 1)
        tasks = connection.execute("SELECT task_key, status, input FROM orchestrator.task_runs WHERE workflow_run_id=%s ORDER BY task_key", (run["id"],)).fetchall()
        assert tasks == [("A", "PENDING", {"n": 1}), ("B", "PENDING", None), ("C", "PENDING", None)]
        assert connection.execute("SELECT count(*) FROM orchestrator.workflow_wakeups WHERE workflow_run_id=%s", (run["id"],)).fetchone()[0] == 1
    null_run = http_api.created("/workflows", {"definition_id": definition["id"]})
    assert connection.execute("SELECT input FROM orchestrator.workflow_runs WHERE id=%s", (null_run["id"],)).fetchone() == (None,)


def test_publish_conflict_and_provider_namespace(http_api, definition_data, connection):
    document = definition_data("sequence.yaml").document()
    barrier = Barrier(2, timeout=5)
    def publish():
        barrier.wait()
        return http_api.request("/workflow-definitions", document)
    with ThreadPoolExecutor(max_workers=2) as pool:
        a, b = [future.result() for future in [pool.submit(publish), pool.submit(publish)]]
    assert sorted([a.status, b.status]) == [201, 409]
    conflict = a if a.status == 409 else b
    assert conflict.body["error"]["code"] == "definition_version_conflict"
    other = http_api.created("/workflow-definitions", dict(document, provider="another"))
    assert other["provider"] == "another"
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 2


@pytest.mark.parametrize("changes,code", [
    ({"tasks": []}, "invalid_definition"),
    ({"name": ""}, "invalid_definition"),
    ({"version": 0}, "invalid_definition"),
    ({"version": "1"}, "invalid_request"),
    ({"unknown": True}, "invalid_request"),
    ({"tasks": [{"id": "A", "type": "activity", "depends_on": ["A"]}]}, "invalid_definition"),
])
def test_invalid_definition_has_no_writes(http_api, definition_data, connection, changes, code):
    response = http_api.request("/workflow-definitions", dict(definition_data("sequence.yaml").document(), **changes))
    assert response.status == (400 if code == "invalid_request" else 422)
    assert response.body["error"]["code"] == code
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0


@pytest.mark.parametrize("definition_id,status,code", [("bad-id", 400, "invalid_request"), ("11111111-1111-4111-8111-111111111111", 404, "definition_not_found")])
def test_start_error_mapping(http_api, connection, definition_id, status, code):
    response = http_api.request("/workflows", {"definition_id": definition_id})
    assert response.status == status and response.body["error"]["code"] == code
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_runs").fetchone()[0] == 0


def test_published_dag_and_retry_cannot_start(http_api, definition_data, connection):
    for filename, code in [("dag.yaml", "unsupported_workflow"), ("sequence.yaml", "unsupported_policy")]:
        document = definition_data(filename).document()
        document["name"] = filename
        if filename == "sequence.yaml":
            document["tasks"][0]["max_attempts"] = 2
        published = http_api.created("/workflow-definitions", document)
        response = http_api.request("/workflows", {"definition_id": published["id"]})
        assert response.status == 422 and response.body["error"]["code"] == code
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_runs").fetchone()[0] == 0


@pytest.mark.parametrize("payload", [r'"\u0000"', r'1e131073', r'"\ud800"'])
def test_jsonb_invalid_payload_rejected_atomically(http_api, definition_data, connection, payload):
    published = http_api.created("/workflow-definitions", definition_data("sequence.yaml").document())
    raw = ('{"definition_id":"'+published["id"]+'","input":'+payload+'}').encode()
    response = http_api.request("/workflows", raw=raw)
    assert response.status == 400 and response.body["error"]["code"] == "invalid_request"
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_runs").fetchone()[0] == 0


@pytest.mark.parametrize("chunked", [False, True])
def test_streaming_body_limit(http_api, connection, chunked):
    response = http_api.request("/workflows", raw=b" "*((1<<20)+1), chunked=chunked)
    assert response.status == 413 and response.body["error"]["code"] == "request_too_large"
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_runs").fetchone()[0] == 0


def test_database_failure_returns_sanitized_error(http_api, connection):
    # An isolated role-independent server-side cancellation error reproduces a storage failure.
    connection.execute("CREATE FUNCTION orchestrator.reject_http_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'secret database detail' USING ERRCODE='57P01'; END $$")
    connection.execute("CREATE TRIGGER reject_http_insert BEFORE INSERT ON orchestrator.workflow_definitions FOR EACH ROW EXECUTE FUNCTION orchestrator.reject_http_insert()")
    response = http_api.request("/workflow-definitions", {"provider":"demo","name":"f","version":1,"tasks":[{"id":"A","type":"activity","depends_on":[]}]})
    assert response.status == 503 and response.body["error"]["code"] == "storage_unavailable"
    assert "secret" not in json.dumps(response.body)
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0


@pytest.mark.parametrize("raw", [b"null", b"{} {}", b"\x0b{}", b"{}\xc2\xa0", b'{"name":"\xff"}'])
def test_invalid_json_over_http_has_no_writes(http_api, connection, raw):
    response = http_api.request("/workflow-definitions", raw=raw)
    assert response.status == 400 and response.body["error"]["code"] == "invalid_request"
    assert connection.execute("SELECT count(*) FROM orchestrator.workflow_definitions").fetchone()[0] == 0
