# Implementation roadmap

Each stage ends with a working scenario and relevant failure checks. Establish the preceding stage's invariants before adding dependent capabilities. Detailed PR descriptions live under [ai/tasks](../tasks/README.md).

| Stage | Scope | Acceptance criterion |
| --- | --- | --- |
| [0. Domain](../tasks/00-domain/README.md) | Immutable definitions, dependencies, pure task/workflow transitions | Invalid transitions, dependency success/failure, and terminal outcomes are verified |
| [1. Durable sequence](../tasks/01-durable-sequence/README.md) | PostgreSQL, migrations, start/inspection API, `A → B → C`, wakeups | Execution resumes from committed data after restart |
| [2. Multi-worker](../tasks/02-multi-worker/README.md) | `SKIP LOCKED` claim and multiple processes | Concurrent claims do not create two valid attempts for one task |
| [3. Lease/recovery](../tasks/03-lease-recovery/README.md) | Heartbeat, lease expiry, attempt-ID fencing | Another worker claims work after a crash; stale reports are rejected |
| [4. Retry](../tasks/04-retry/README.md) | Attempt history, error types, backoff and jitter, attempt limits | History is complete; exhausted attempts lead to FAILED workflow |
| [5. DAG](../tasks/05-dag/README.md) | Acyclicity, independent parallel steps, joins | READY requires success of all mandatory dependencies |
| [6. Timers/timeouts](../tasks/06-timers-timeouts/README.md) | Durable timers, task and workflow deadlines | Waiting survives restart; deadlines yield terminal outcomes |
| [7. External events](../tasks/07-external-events/README.md) | WAITING, correlation, deduplication, event ingestion | Duplicate events do not activate the next step twice |
| [8. API idempotency](../tasks/08-api-idempotency/README.md) | Start keys and report deduplication | Repeated requests return the original run |
| [9. Observability](../tasks/09-observability/README.md) | Structured logs, metrics, tracing | Every retry and stalled workflow can be explained |
| [10. Control](../tasks/10-control/README.md) | Cancellation followed by pause/resume | Pending cancellation and cooperative stopping of running work are verified |

## First useful release

The [MVP contract](../../docs/decisions/0001-mvp-contract.md) establishes attempts, leases, and fenced completion from stage 1 with a one-attempt limit. Stage 3 adds heartbeat/recovery; stage 4 enables larger finite limits.

Stages 0–4 deliver sequential workflows, persisted execution, multiple workers, leases, recovery, and bounded retries. API: `POST /workflow-definitions`, `POST /workflows`, `GET /workflows/{id}`, `GET /workflows/{id}/tasks`. Publish changed definitions as new immutable versions.

Demonstration: create three steps, interrupt a worker after an external effect, expire its lease, retry the activity with a stable idempotency key, and inspect one logical result and both attempts through the API. If the external service does not support a key, the demonstration activity implements the check itself. Configure mocked time before automated execution and explicitly advance it through expiry.

## Failure checks

| Failure | Expected result |
| --- | --- |
| Worker dies after claim | The task becomes available after lease expiry |
| Worker dies after external effect but before commit | Retry uses the same idempotency key |
| Stale worker reports after a new claim | The obsolete attempt ID is rejected |
| Engine dies while processing a wakeup | Transaction rolls back; wakeup remains |
| Two schedulers recover one lease | One recovery transition and one active attempt after claim |
| PostgreSQL is temporarily unavailable | Uncommitted changes are not treated as durable; processing resumes |
| An event arrives twice | Deduplication yields one workflow continuation |

Verify each scenario in the stage that introduces its behavior. Write integration and failure tests in Python against real PostgreSQL. Set up reusable mocks and fake clocks before running temporal scenarios; synchronize concurrency with barriers rather than sleeps. Limit PRs to 500 changed business-logic lines, excluding tests and documentation. Contract/data changes always update references and `AGENTS.md`.

See [project context](01-context.md), [code structure](02-code-structure.md), and [data model](03-data-model.md).
