# Data model and state transitions

## Entities

| Table | Purpose | Fields and constraints to define |
| --- | --- | --- |
| `workflow_definitions` | Immutable published definition | `id, name, version, provider, definition jsonb, created_at`; `UNIQUE(provider, name, version)` |
| `workflow_runs` | Execution of a specific definition version | `id, definition_id, status, input, output, version, created_at, started_at, finished_at, deadline_at` |
| `task_runs` | Logical step | `id, workflow_run_id, task_key, status, input, output, max_attempt_count, attempt_count, available_at, lease_owner, lease_expires_at, current_attempt_id`; `UNIQUE(workflow_run_id, task_key)` |
| `task_attempts` | One execution attempt | `id, task_run_id, attempt_no, worker_id, status, started_at, heartbeat_at, finished_at, error_type, error_message`; `UNIQUE(task_run_id, attempt_no)` |
| `workflow_wakeups` | Request to reevaluate a workflow | `workflow_run_id PRIMARY KEY, created_at` |

### Canonical MVP contract

The [MVP decision](../../docs/decisions/0001-mvp-contract.md) defines the fields, states, HTTP errors, and race scenarios before migrations. Definition versions are immutable and unique by `(provider, name, version)`; `(provider, name)` alone is not unique. Empty definitions are invalid. Publication supports valid DAGs; execution accepts only a single complete chain until stage 5.

`current_attempt_id` points to this task's active attempt and is non-null exactly while RUNNING, alongside owner and expiry; clear all three on closure. `attempt_count` starts at zero and increments only with a committed claim/attempt insertion; its new value is `attempt_no`. Definition `max_attempts` (default 1, positive signed 32-bit integer) is copied without conversion to `max_attempt_count`. Stages 1–3 start only policies with limit 1; stage 4 enables larger finite limits.

`workflow_runs.version` is a concurrency revision, initially 1, incremented once per committed transaction changing the existing run or its tasks/attempts, including claims/heartbeats. Wakeup-only changes and no-op evaluations do not increment it. It is not the immutable definition version.

Start creates a PENDING run/tasks and a wakeup. Evaluation starts the run and activates its root. Root input equals run input (omitted means JSON null); a successor receives its predecessor's complete output atomically when becoming READY. Inputs stay fixed through retries. Final-step output becomes successful run output; failures leave run output null and unstarted successors PENDING. Status distinguishes unavailable output from successful JSON null.

`workflow_runs.definition_id` references exactly one immutable definition version. A separate `definition_version` field is unnecessary and must not diverge from the definition row. Materialize task_runs from that fixed definition in the start transaction.

```json
{
  "provider": "demo",
  "name": "resize_database",
  "version": 1,
  "tasks": [
    {"id": "validate", "type": "activity", "depends_on": []},
    {"id": "resize", "type": "activity", "depends_on": ["validate"]},
    {"id": "wait_replication", "type": "activity", "depends_on": ["resize"]}
  ]
}
```

Publication validates nonempty case-sensitive identities, positive version, nonempty tasks, activity type, unique task IDs, unique existing dependencies, no self-dependencies, and acyclicity across all components. The publication boundary owns the nonempty task-ID and activity-type checks; the domain constructor does not enforce them. The pure domain constructor snapshots all mutable task data and exposes detached copies; default max_attempts is resolved to 1 without retaining the input policy pointer. Retry and timeout policies belong to the immutable definition version; changing the definition requires a new version.

## States

- Workflow: `PENDING → RUNNING → SUCCEEDED | FAILED | CANCELLED | TIMED_OUT`. Each operation defines the terminal outcomes it supports.
- Task: `PENDING → READY → RUNNING → SUCCEEDED`. Transient failure: `RUNNING → RETRY_WAIT → READY`. Exhausted attempts: `RUNNING → FAILED`. FAILED means a final logical-task failure, not an intermediate retry.
- Attempt: `RUNNING → SUCCEEDED | FAILED | TIMED_OUT | LOST_LEASE`. Attempt closure and task_run changes are atomic.

Lease expiry closes the active attempt and moves the task to READY/RETRY_WAIT or FAILED according to its finite policy. External event handling defines WAITING transitions in its own stage.

`task.State.LastAttempt` is an outcome snapshot, not `current_attempt_id` or
stored attempt history. Storage must still clear the active token and lease on
closure. The one-attempt policy remains in force until stage 4.

## Task claim

```sql
-- Candidate workflow is already locked; sample database time after locking.
SELECT id FROM task_runs
WHERE workflow_run_id = $1
  AND status = 'READY' AND available_at <= $2
  AND attempt_count < max_attempt_count
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT $3;
```

Discover candidates without row locks, then lock their workflow with `FOR UPDATE SKIP LOCKED` and recheck that it is RUNNING before this query. In the same transaction, update `status, attempt_count, current_attempt_id, lease_owner, lease_expires_at` and create a task_attempt. Commit before executing the activity. Heartbeat and completion validate `current_attempt_id, lease_owner, status = 'RUNNING'` and an unexpired lease. Reject stale worker reports. Index `(available_at, id) WHERE status = 'READY'` and `(lease_expires_at) WHERE status = 'RUNNING'`.

Bind `$2` to authoritative database time sampled after acquiring the workflow lock, not transaction-start `now()`. Equality with lease expiry is expired. The SQL example uses production database time. Before running time-dependent tests, mock the authoritative time source through an isolated test seam, including SQL time, and align it with the application clock. Do not assume a mocked Go clock changes `now()` in PostgreSQL. Keep transactions and locks real in Python integration tests.

## Workflow reevaluation

Pure evaluation rejects inconsistent task snapshots before proposing changes.
Terminal runs need no task inspection to consume redundant wakeups; their stored
result and timestamps must remain unchanged.

Update a task and execute `INSERT workflow_wakeups ... ON CONFLICT DO NOTHING` in one transaction. Every writer locks workflow → wakeup (if present) → tasks by ID → attempts by ID, using READ COMMITTED and one workflow per transaction. Discovery reads do not lock wakeups or tasks first. The engine locks the workflow before its wakeup, rereads tasks, evaluates, and deletes the signal atomically. Completion either commits before evaluation sees it or waits and recreates the signal after evaluation commits. Missing wakeup rows are protected by the workflow lock. Claims and heartbeats follow the same order; no external call occurs inside the transaction.

Whenever the data model or a contract changes, update the affected reference documents and `AGENTS.md` in the same PR.

See the [roadmap](04-roadmap.md) and [implementation tasks](../tasks/README.md).
