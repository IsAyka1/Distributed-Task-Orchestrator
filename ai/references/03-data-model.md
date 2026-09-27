# Data model and state transitions

## Entities

| Table | Purpose | Fields and constraints to define |
| --- | --- | --- |
| `workflow_definitions` | Immutable published definition | `id, name, version, provider, definition jsonb, created_at`; version uniqueness within the chosen definition namespace |
| `workflow_runs` | Execution of a specific definition version | `id, definition_id, status, input, output, version, created_at, started_at, finished_at, deadline_at` |
| `task_runs` | Logical step | `id, workflow_run_id, task_key, status, input, output, max_attempt_count, attempt_count, available_at, lease_owner, lease_expires_at, current_attempt_id`; `UNIQUE(workflow_run_id, task_key)` |
| `task_attempts` | One execution attempt | `id, task_run_id, attempt_no, worker_id, status, started_at, heartbeat_at, finished_at, error_type, error_message`; `UNIQUE(task_run_id, attempt_no)` |
| `workflow_wakeups` | Request to reevaluate a workflow | `workflow_run_id PRIMARY KEY, created_at` |

### Contract decisions before migrations

[Task 00-01](../tasks/00-domain/01-task.md) defines the definition namespace and version uniqueness. Compare `UNIQUE(name, version)` with provider-scoped version uniqueness; a standalone `UNIQUE(provider, name)` on version rows would prevent publishing multiple versions. Choose constraints that preserve immutable version publication.

Use `current_attempt_id` consistently for the attempt FK; avoid a competing `current_attempt` alias. Define the mapping between policy `max_attempts` and stored `max_attempt_count`, the maintenance of `attempt_count`, and the meaning of `workflow_runs.version` in the same contract decision. The table above uses these identifiers for discussion; the decision must synchronize schema, code, and references before migrations.

`workflow_runs.definition_id` references exactly one immutable definition version. A separate `definition_version` field is unnecessary and must not diverge from the definition row. Materialize task_runs from that fixed definition in the start transaction.

```json
{
  "name": "resize_database",
  "version": 1,
  "tasks": [
    {"id": "validate", "type": "activity", "depends_on": []},
    {"id": "resize", "type": "activity", "depends_on": ["validate"]},
    {"id": "wait_replication", "type": "activity", "depends_on": ["resize"]}
  ]
}
```

Publication validates unique task IDs, existing dependencies, and acyclicity. Retry and timeout policies belong to the immutable definition version; changing the definition requires a new version.

## States

- Workflow: `PENDING → RUNNING → SUCCEEDED | FAILED | CANCELLED | TIMED_OUT`. Each operation defines the terminal outcomes it supports.
- Task: `PENDING → READY → RUNNING → SUCCEEDED`. Transient failure: `RUNNING → RETRY_WAIT → READY`. Exhausted attempts: `RUNNING → FAILED`. FAILED means a final logical-task failure, not an intermediate retry.
- Attempt: `RUNNING → SUCCEEDED | FAILED | TIMED_OUT | LOST_LEASE`. Attempt closure and task_run changes are atomic.

Lease expiry closes the active attempt and moves the task to READY/RETRY_WAIT or FAILED according to its finite policy. External event handling defines WAITING transitions in its own stage.

## Task claim

```sql
SELECT id FROM task_runs
WHERE status = 'READY' AND available_at <= now()
ORDER BY available_at, id
FOR UPDATE SKIP LOCKED
LIMIT $1;
```

In the same transaction, update `status, attempt_count, current_attempt_id, lease_owner, lease_expires_at` and create a task_attempt. Commit before executing the activity. Heartbeat and completion validate `current_attempt_id, lease_owner, status = 'RUNNING'` and an unexpired lease. Reject stale worker reports. Index `(available_at, id) WHERE status = 'READY'` and `(lease_expires_at) WHERE status = 'RUNNING'`.

The SQL example uses production database time. Before running time-dependent tests, mock the authoritative time source through an isolated test seam, including SQL time, and align it with the application clock. Do not assume a mocked Go clock changes `now()` in PostgreSQL. Keep transactions and locks real in Python integration tests.

## Workflow reevaluation

Update a task and execute `INSERT workflow_wakeups ... ON CONFLICT DO NOTHING` in one transaction. The engine locks the wakeup, evaluates the workflow, and deletes the wakeup atomically. Establish a consistent lock order across all writers. Serialize evaluation per workflow and ensure concurrent events cannot lose the newest persisted changes; a revision counter is an option for long evaluations.

Whenever the data model or a contract changes, update the affected reference documents and `AGENTS.md` in the same PR.

See the [roadmap](04-roadmap.md) and [implementation tasks](../tasks/README.md).
