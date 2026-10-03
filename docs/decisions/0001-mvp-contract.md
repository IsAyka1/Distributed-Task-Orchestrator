# 0001. MVP execution contract

Task: [00-01](../../ai/tasks/00-domain/01-task.md). Scope: sequential execution in stages 0–4. This decision is the contract for domain types, application use cases, PostgreSQL storage, HTTP clients, workers, and their tests. It specifies behavior to implement; it does not claim that an implementation exists.

## Definitions and identity

A published definition has a generated UUID `id`, required nonempty case-sensitive strings `provider` and `name`, a positive integer `version`, and a nonempty array `tasks`. Each task has a required nonempty string `id` (the task key), `type: "activity"`, an array of task-key strings `depends_on`, and optional integer `max_attempts` (default 1). The task key also identifies its registered activity in the MVP. Reject duplicate task keys, duplicate dependencies, unknown dependencies, self-dependencies, and cycles. Do not trim or case-fold identity fields.

Publication uniqueness is `UNIQUE(name, version, provider)`. There is no standalone uniqueness constraint on `(provider, name)`. Versions need not be consecutive. A second publication of the same tuple conflicts even if the content matches; no update or delete operation is provided. Different providers may publish the same name/version. A run's `definition_id` selects exactly one immutable version; there is no “latest” resolution and no duplicated definition-version column on the run.

Reject an empty definition at publication and in domain validation. Valid DAGs may be published, but starting one before stage 5 returns `unsupported_workflow`. A supported sequence has exactly one root, each other task depends on exactly one predecessor, each task has at most one successor, and the chain contains every task. A single task is a valid sequence. Execution order follows dependencies, not declaration order.

Known application records use explicit named types, including definition, task definition, workflow run, task run, attempt, claim, completion, and HTTP error. JSON is decoded and validated into these types at boundaries. Activity-specific input/output is an opaque JSON value to the orchestrator, carried in a named payload type; activity adapters validate it into their own named types. This does not authorize generic maps for known orchestrator fields.

Payloads must fit PostgreSQL JSONB: NUL characters, invalid Unicode surrogate
escapes and numbers outside the database numeric range are rejected as
`invalid_request`, without exposing SQL errors or payload contents. JSONB
normalization applies; raw formatting and duplicate object keys are not preserved.

Example publication (three registered activities):

```json
{
  "provider": "demo",
  "name": "sequence",
  "version": 1,
  "tasks": [
    {"id": "A", "type": "activity", "depends_on": [], "max_attempts": 1},
    {"id": "B", "type": "activity", "depends_on": ["A"], "max_attempts": 1},
    {"id": "C", "type": "activity", "depends_on": ["B"], "max_attempts": 1}
  ]
}
```

## Canonical runtime fields

All entity IDs are UUIDs. Timestamps represent UTC instants. Optional timestamps and IDs are nullable, not empty strings or zero UUIDs.

| Field | Meaning and maintenance |
| --- | --- |
| `task_runs.max_attempt_count` | Immutable copy of definition `max_attempts`, including the first attempt. Positive signed 32-bit integer; zero, negative, fractional, and out-of-range values are invalid. Defaults to 1; never means unlimited. Stages 1–3 accept only 1 on start; stage 4 enables larger limits. |
| `task_runs.attempt_count` | Starts at 0. Increment exactly once in the claim transaction that inserts an attempt. Rollback consumes no number. Never decrement; never exceed `max_attempt_count`. |
| `task_attempts.attempt_no` | New `attempt_count`, starting at 1; unique per task. Failed and lost attempts count toward the limit. |
| `task_runs.current_attempt_id` | Nullable FK to the active attempt belonging to this task, set on claim and cleared on closure. Non-null exactly while RUNNING. Enforce same-task ownership in storage, not just an unconstrained UUID reference. Historical attempts remain queryable. |
| `task_runs.lease_owner`, `lease_expires_at` | Non-null exactly while RUNNING; clear atomically with `current_attempt_id` on closure. Owner is the claiming worker identity. |
| `workflow_runs.version` | Positive signed 64-bit concurrency revision, initially 1. Increment once per committed transaction that changes the existing run or any of its task/attempt rows (including claim and heartbeat), irrespective of row count. Wakeup-only changes and no-op evaluation do not increment. Failed transactions leave it unchanged. It is unrelated to definition version and is not a client-supplied field. |

Do not alias `current_attempt_id` as `current_attempt`. Claim requires READY, `available_at <= database_time`, a RUNNING workflow, and remaining attempts. Stage 1 already creates attempts and leases and fences completion; heartbeat/recovery loops are introduced in stage 3, retry scheduling in stage 4. Stage-1 demonstrations finish each activity within its lease and restart only between attempts. An expired RUNNING attempt may remain stuck until recovery exists; expiry does not silently bypass fencing.

## States and value propagation

`StartWorkflow` atomically creates a PENDING workflow, PENDING tasks, and a wakeup. First evaluation sets the workflow RUNNING and the root READY. Root input is the supplied workflow input; omitted input means JSON `null`. Other task inputs initially contain JSON `null` and become the predecessor's complete successful output in the same transaction that makes them READY. No merge, interpolation, or implicit wrapping occurs. Input is frozen once READY, including across retries.

Successful completion atomically sets the task and attempt SUCCEEDED and stores the activity's output; a successful JSON `null` is valid. Output is unavailable until success, so status distinguishes an absent result from a successful null. Failed attempts must not publish a successful output. The run's output stays JSON `null` until the final task succeeds, then becomes that task's full output when evaluation marks the run SUCCEEDED.

| Entity | Baseline transitions and restrictions |
| --- | --- |
| Workflow | PENDING → RUNNING → SUCCEEDED or FAILED. Terminal status never changes. Cancellation and timeout states are reserved for their later stages. |
| Task | PENDING → READY → RUNNING → SUCCEEDED or FAILED. FAILED is final, never an intermediate failed attempt. A failed task makes evaluation mark the workflow FAILED; unstarted successors remain PENDING and cannot be claimed under the terminal workflow. |
| Attempt | RUNNING → SUCCEEDED or FAILED in stage 1. Stage 3 may close it LOST_LEASE; task becomes FAILED at the one-attempt limit. TIMED_OUT is reserved for timeout support. |

Stage 4 adds RUNNING → RETRY_WAIT → READY for retryable failures with budget remaining. Closing an attempt, clearing its lease/token, changing the task, and enqueuing reevaluation are atomic. Permanent failures or exhausted budgets make the task FAILED. Illegal transitions and rejected reports do not mutate any entity. Set `started_at` when entering RUNNING and `finished_at` at terminal transition; timestamps are not reset by a repeated evaluation. A repeated completion is rejected in the baseline; report deduplication belongs to stage 8.

## Transactions, leases, and lock order

Use short application-owned transactions at READ COMMITTED isolation. Every writer of existing execution state follows this lock order: **workflow row → wakeup row → task rows ordered by ID → attempt rows ordered by ID**. Acquire the workflow row even for claim and heartbeat; this serializes writes within a workflow. A missing wakeup row needs no placeholder lock: its insert/delete is protected by the workflow lock. A transaction may omit rows it does not need but cannot acquire an earlier category after a later one. Process one workflow per transaction; discovery reads acquire no row locks and their candidates must be rechecked after locking.

An evaluator discovers workflow IDs without locking wakeups, locks the workflow first, then its wakeup and tasks, rereads their committed state, applies evaluation, and deletes the wakeup in the same transaction. Terminal runs consume redundant wakeups without changing terminal state.

Claim discovery likewise acquires no task lock first. Lock the candidate workflow with `FOR UPDATE SKIP LOCKED`, then claim eligible task rows with `FOR UPDATE SKIP LOCKED`, rechecking status, availability, and budget. Insert the attempt, increment counters/revision, and set token/owner/expiry atomically. Commit before returning work or making an external call. This intentionally trades per-workflow write concurrency for a single lock protocol; it still permits independent workflows to progress concurrently.

Completion locks in the same order, validates RUNNING task/workflow, active attempt identity, owner, and a strictly unexpired lease, closes the attempt and updates the task, increments the run revision, and inserts the wakeup with `ON CONFLICT DO NOTHING`, all before commit. Heartbeat and recovery use the same fencing and lock protocol when introduced. At `database_time == lease_expires_at` the lease is expired. Sample authoritative database time after acquiring locks for the guarded mutation, rather than using a transaction-start timestamp that may precede a lock wait. Use the same sampled instant for that mutation's expiry checks and timestamps.

The application must not call external activities inside a transaction. Every attempt for a logical task uses the same `task_run_id` as its external idempotency key; attempts remain at-least-once. Controlled application/database time in tests is configured before execution; production cannot enable the test time seam.

### Complete/Evaluate race review

| Ordering | Result |
| --- | --- |
| Complete obtains workflow lock first | It commits result and wakeup together. Evaluate subsequently reads the new result before removing the signal. |
| Evaluate obtains workflow lock first | Complete waits. Evaluate may remove the old signal; Complete then commits its result and inserts a new signal. |
| Two evaluators select the same candidate | Workflow locking serializes them; the second rechecks the wakeup and performs no work if it is absent. |
| Either transaction fails before commit | Its state changes, revision increment, and wakeup mutation all roll back together. |

These are acceptance scenarios for real PostgreSQL tests in task 01-06, not a substitute for those tests.

## HTTP boundary and errors

The MVP has no authentication or tenant authorization. `provider` is a naming namespace, not an authorization boundary. Worker claim, completion, and heartbeat remain internal use cases. No public endpoints for them are added by this contract.

| Operation | Request and success |
| --- | --- |
| `POST /workflow-definitions` | Definition fields above; server generates ID/time. `201` returns `id`, `provider`, `name`, `version`. |
| `POST /workflows` | Required UUID `definition_id`, optional JSON `input`; `201` returns `id` and `status: "PENDING"` captured at creation. Every keyless request creates a distinct run. |
| `GET /workflows/{id}` | `200` returns run `id`, `definition_id`, `status`, `input`, `output`, concurrency `version`, `created_at`, nullable `started_at` and `finished_at`. |
| `GET /workflows/{id}/tasks` | `200` returns an object with `tasks`, the full array in dependency order, each containing `id`, `task_key`, `status`, `input`, `output`, `attempt_count`, `max_attempt_count`, nullable `current_attempt_id`. Attempt history is added in stage 4. |

Request bodies are UTF-8 JSON objects, limited to 1 MiB; enforce the limit while reading, including chunked requests. Require `application/json` (optional charset allowed), reject malformed/trailing JSON and unknown orchestrator fields, and validate named DTOs before starting a write transaction. Payload fields remain opaque JSON. UUIDs are serialized as strings and timestamps as RFC 3339 UTC strings. Successful writes respond only after commit. GET responses read committed data with a consistent snapshot per response; separate requests need not share a snapshot. Clients must tolerate additive response fields.

Errors use a named `ErrorResponse` containing an `ErrorDetail` with required string fields `code` and `message`. Messages are safe explanations, not a stable parsing interface. For example:

```json
{"error":{"code":"definition_version_conflict","message":"This definition version already exists."}}
```

| HTTP | Stable code | Condition / retry semantics |
| --- | --- | --- |
| 400 | `invalid_request` | Malformed JSON, unknown fields, invalid UUID, missing/wrongly typed required fields; correct request first. |
| 413 | `request_too_large` | Request exceeds 1 MiB; reduce body. |
| 415 | `unsupported_media_type` | Wrong or missing content type. |
| 422 | `invalid_definition` | Invalid graph, empty tasks, invalid identity or attempt policy. |
| 422 | `unsupported_workflow` | Valid graph is not a sequence at this stage. |
| 422 | `unsupported_policy` | Start requests a limit other than 1 before stage 4. |
| 404 | `definition_not_found` / `workflow_not_found` | Well-formed ID has no matching resource. |
| 409 | `definition_version_conflict` | Publication tuple already exists; choose a new version. |
| 503 | `storage_unavailable` | Temporary database failure; GET may be retried. A POST with unknown commit outcome must not be blindly retried because baseline starts are not idempotent. |
| 500 | `internal_error` | Unexpected failure; disclose no SQL, driver errors, payloads, or stack traces. |

Internal rejected reports return typed `stale_attempt`; illegal domain events return `invalid_transition`, with no mutation. These are not public HTTP operations. Auth, pagination of future history, deadline policies, DAG input merging, and API idempotency are left to their designated stages rather than implicitly supported here.

## Review scenarios and compatibility

| Scenario | Required observation |
| --- | --- |
| Publish demo/sequence v1 and v2 | Both receive distinct IDs; republishing v1 conflicts; existing v1 runs retain v1. Another provider can publish sequence v1. |
| Start A → B → C with input `{"n":1}` | A receives that value. If A outputs `{"n":2}`, B receives exactly that; if B outputs `[3]`, C receives `[3]`. C output `null` yields a SUCCEEDED workflow with output `null`. |
| B fails at its limit | B task/attempt become FAILED together; evaluation fails the run, C stays PENDING, no C activity executes, run output is null. |
| Empty / branching definition | Empty is rejected on publish; acyclic branching can publish but cannot start before stage 5. |
| Claim rollback / expired report | Rollback leaves count/token/attempt/revision unchanged; report at exact expiry is rejected without a wakeup or result. |
| Concurrent Complete/Evaluate | Both lock orderings preserve either observed completion or a pending wakeup, as detailed above. |

This is the initial contract in a documentation-only repository; there are no deployed schema or API consumers to migrate. Future changes must update this decision, affected references, AGENTS.md, and dependent task acceptance criteria. Stage 00-02 supplies build commands; stages 00-03–00-05 implement domain checks; stage 1 supplies real database/HTTP checks. Until those exist, verification here consists of JSON example parsing, local link checks, and contract/scenario consistency review.
