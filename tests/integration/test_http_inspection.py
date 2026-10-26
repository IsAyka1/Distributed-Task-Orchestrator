from uuid import UUID

import pytest


def get(client, run_id, suffix=""):
    response = client.request(f"/workflows/{run_id}{suffix}", method="GET", raw=b"", media=None)
    assert response.status == 200, response
    assert response.content_type == "application/json"
    return response.body


def start_sequence(client):
    # Neither declaration order nor lexical order matches dependency order.
    published = client.created("/workflow-definitions", {
        "provider": "demo", "name": "inspection", "version": 1,
        "tasks": [
            {"id": "last", "type": "activity", "depends_on": ["alpha"]},
            {"id": "zeta", "type": "activity", "depends_on": []},
            {"id": "alpha", "type": "activity", "depends_on": ["zeta"]},
        ],
    })
    run = client.created("/workflows", {"definition_id": published["id"], "input": {"n": 1}})
    return published["id"], run["id"]


def evaluate(workflows, run_id):
    result = workflows.evaluate(run_id)
    assert result.returncode == 0, result.stderr


@pytest.mark.parametrize("final_output", [None, {"result": [3, True]}])
def test_inspect_committed_sequence(http_api, workflows, claims, completions, clock, final_output):
    definition_id, run_id = start_sequence(http_api)
    run = get(http_api, run_id)
    assert run == {
        "id": run_id, "definition_id": definition_id, "status": "PENDING", "input": {"n": 1},
        "output": None, "version": 1, "created_at": clock.now.isoformat().replace("+00:00", "Z"),
        "started_at": None, "finished_at": None,
    }
    tasks = get(http_api, run_id, "/tasks")["tasks"]
    assert [v["task_key"] for v in tasks] == ["zeta", "alpha", "last"]
    ids = [v["id"] for v in tasks]
    assert len(set(ids)) == 3
    for i, value in enumerate(tasks):
        assert UUID(value["id"]).version == 4
        assert value == {"id": ids[i], "task_key": ["zeta", "alpha", "last"][i], "status": "PENDING",
                         "input": {"n": 1} if i == 0 else None, "output": None,
                         "attempt_count": 0, "max_attempt_count": 1, "current_attempt_id": None}
    revision = 1
    for i, output in enumerate([{"n": 2}, [3], final_output]):
        evaluate(workflows, run_id)
        assert get(http_api, run_id, "/tasks")["tasks"][i]["status"] == "READY"
        claim = claims.claim()
        active = get(http_api, run_id, "/tasks")["tasks"][i]
        assert active["status"] == "RUNNING" and active["attempt_count"] == 1
        assert active["current_attempt_id"] == claim["Attempt"]["ID"]
        assert active["id"] == ids[i] == claim["TaskID"]
        completions.complete(claim, output=output)
        closed = get(http_api, run_id, "/tasks")["tasks"][i]
        assert closed["status"] == "SUCCEEDED" and closed["output"] == output
        assert closed["current_attempt_id"] is None
        run = get(http_api, run_id)
        assert run["version"] > revision
        revision = run["version"]
    evaluate(workflows, run_id)
    run = get(http_api, run_id)
    assert run["status"] == "SUCCEEDED" and run["output"] == final_output
    assert run["version"] > revision
    assert run["started_at"] == run["finished_at"] == run["created_at"]
    finished = get(http_api, run_id, "/tasks")["tasks"]
    assert [v["id"] for v in finished] == ids
    assert [v["input"] for v in finished] == [{"n": 1}, {"n": 2}, [3]]


def test_inspect_failed_sequence(http_api, workflows, claims, completions):
    _, run_id = start_sequence(http_api)
    evaluate(workflows, run_id)
    completions.complete(claims.claim(), event="FAIL", error_message="private activity failure")
    evaluate(workflows, run_id)
    run = get(http_api, run_id)
    tasks = get(http_api, run_id, "/tasks")["tasks"]
    assert run["status"] == "FAILED" and run["output"] is None and run["finished_at"] is not None
    assert [v["status"] for v in tasks] == ["FAILED", "PENDING", "PENDING"]
    assert all(v["current_attempt_id"] is None for v in tasks)
    assert "private activity failure" not in str(tasks)


@pytest.mark.parametrize("suffix", ["", "/tasks"])
@pytest.mark.parametrize("run_id,status,code", [
    ("not-a-uuid", 400, "invalid_request"),
    ("%00", 400, "invalid_request"),
    ("%FF", 400, "invalid_request"),
    ("11111111-1111-4111-8111-111111111111", 404, "workflow_not_found"),
])
def test_inspection_id_errors(http_api, suffix, run_id, status, code):
    response = http_api.request(f"/workflows/{run_id}{suffix}", method="GET", raw=b"", media=None)
    assert response.status == status and response.body["error"]["code"] == code
    assert set(response.body) == {"error"}


def test_postgres_uuid_forms_return_canonical_id(http_api):
    _, run_id = start_sequence(http_api)
    assert get(http_api, run_id.replace("-", "").upper())["id"] == run_id
    assert len(get(http_api, run_id.upper(), "/tasks")["tasks"]) == 3
