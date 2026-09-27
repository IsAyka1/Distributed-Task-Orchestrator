# Implementation tasks

Build the Distributed Task Orchestrator in small, independently verifiable PRs. Use the [references](../references/README.md) for requirements and [AGENTS.md](../../AGENTS.md) for project rules.

## Scope and contract decisions

- The architecture is a modular Go monolith with PostgreSQL. Stages 0–4 define the first useful release. Deployment, Kafka, leader election, Saga, replay, reset, UI, and authentication require separate scope.
- Stage 1 establishes baseline attempts, leases, fencing, and atomic claim. Stage 2 verifies multiple processes; stage 3 adds heartbeat/recovery; stage 4 adds retry policy and API history. Stage-1 restart checks happen between attempts; recovery tests cover interrupted RUNNING attempts in stage 3.
- [00-01](00-domain/01-task.md) records the [MVP contract](../../docs/decisions/0001-mvp-contract.md), which defines canonical attempt fields, finite limits, run.version, immutable definition uniqueness, input/output propagation, and lock order. Keep these choices synchronized with the data reference rather than resolving them implicitly in SQL.
- Definition validation checks cycles in stage 0. Stage 5 enables DAG execution. Dedicated contract tasks define branch-failure/input merging, timer deadlines, early events, idempotency namespaces, and pause/resume.
- All writers follow a consistent workflow/wakeup/task lock order. Test lost-wakeup races in stage 1 even when its demonstration uses one process.
- Select and pin compatible Go/PostgreSQL/tool versions during bootstrap. Establish reusable clocks/mocks and the Python integration harness before dependent behavior is implemented.

## Stages and dependencies

Follow `00 → 01 → … → 10` and each stage's task order. This is a conservative integration sequence. Begin a dependent stage only after the preceding stage meets its acceptance criteria.

| Stage | PR tasks | Observable outcome |
| --- | --- | --- |
| [0. Domain](00-domain/README.md) | 5 | Pure domain logic validates definitions, transitions, and sequential workflow outcomes. |
| [1. Durable sequence](01-durable-sequence/README.md) | 12 | A single process executes A → B → C through PostgreSQL and resumes from committed progress after restart. |
| [2. Multi-worker](02-multi-worker/README.md) | 3 | Multiple processes share the queue without two valid attempts for one task. |
| [3. Lease/recovery](03-lease-recovery/README.md) | 4 | Work recovers after a worker crash, and stale attempts cannot persist results. |
| [4. Retry](04-retry/README.md) | 6 | Bounded retries preserve complete history and end in success or FAILED. |
| [5. DAG](05-dag/README.md) | 3 | Independent branches execute concurrently, and joins wait for every mandatory dependency. |
| [6. Timers/timeouts](06-timers-timeouts/README.md) | 5 | Waiting and deadlines are durable; restart does not reset time or terminal outcomes. |
| [7. External events](07-external-events/README.md) | 5 | WAITING persists, events correlate correctly, and duplicate delivery produces one stored continuation. |
| [8. API idempotency](08-api-idempotency/README.md) | 3 | Repeated start requests return the original run, and repeated reports cannot cause additional transitions. |
| [9. Observability](09-observability/README.md) | 4 | Telemetry and history explain every retry and reason for missing progress. |
| [10. Control](10-control/README.md) | 5 | Cancellation and pause/resume persist across restarts and cannot revive terminal workflows. |

## Execution rules

- Before taking a task, inspect the working tree, preserve local changes, and always run `git pull --ff-only` on the intended base branch. Resolve pull failures or conflicts before implementation without discarding local work. Then create a new dedicated branch using `<task-id>-<short-description>` by default; do not include `codex` anywhere in branch names. Never implement a task on the base branch or reuse another task's branch.
- After completing a task and meeting its acceptance criteria, required checks, and PR requirements, always mark its task file with `Status: Done`, check the satisfied Definition of Done items, and record the PR link and verification results. Update any existing status entry or checklist for that task in its stage README or this index. Leave incomplete or blocked tasks open.
- After the task and checks are complete, commit only the task changes, push the branch, and open a pull request against its intended base. Include the task reference, outcome, verification results, and business-logic line count. Return the PR URL; the task is not complete without an open PR. Report access/authentication blockers explicitly. Do not merge unless separately instructed.
- One task means one PR and one observable outcome. Limit the PR to **500 changed business-logic lines**, counting additions + deletions against its base. Aim for 200–400; tests and documentation are excluded.
- Count behavior-bearing production code, SQL, and migrations as business logic. Exclude non-business scaffolding and generated metadata. Classify mixed files by changed lines rather than excluding a whole file that contains business behavior. Show the counting basis in the PR.
- Estimates guide scoping. A zero estimate means the task adds no business logic; its documentation and tests still require review. Reestimate against the code before implementation. Split oversized work into tasks with their own DoD, checks, and dependencies.
- Keep mandatory regression tests in the same PR as their behavior changes. Do not compress code or reduce test coverage to satisfy the limit. Excluded files must remain relevant and reviewable.
- Each PR must build and pass applicable checks. Do not expose an incomplete capability through the API. Additive schema/contracts can be independently verified intermediate PRs.
- Write Go unit tests and **Python integration and failure tests**. Use shared Python database/process fixtures and reusable mocks/fakes. Verify transactions, locks, rollback, and concurrency against isolated real PostgreSQL; mocking the database cannot establish these guarantees.
- **Mock time in advance:** configure the clock's initial instant, timers/tickers, and deterministic jitter before constructing or running a time-dependent component. Advance fake time explicitly. For SQL tests, configure an isolated database-time seam and align application/database clocks before the scenario. Keep production time authoritative in PostgreSQL and test controls inaccessible in production.
- Use reusable barriers for races. Bound harness execution with wall-clock timeouts, but do not use sleeps or wall-clock passage to decide business outcomes. Reset shared mocks and clocks between tests to prevent leakage.
- Run formatting checks, affected Go unit tests, `go test ./...`, and the relevant build. Run race detection for concurrent Go code and the documented Python integration/failure commands. Missing database access means an unexecuted check, not a passing result.
- Whenever a contract or data model changes, **always update the affected `ai/references` files and `AGENTS.md` in the same PR**. Update task scope and criteria as needed. Keep enduring requirements separate from implementation-progress notes.
- Read dependencies, relevant source, and contract decisions before implementation. Resolve product choices before dependent code. Deployment or production-data operations are not implied by these tasks.
- DoD: acceptance criteria and checks pass, verification commands/results are recorded in the PR, the diff is reviewed, the business-logic limit is checked, required docs are updated, migration compatibility is evaluated, and a pull request is open from the dedicated task branch. Documentation-only PRs require example/link/consistency checks rather than application tests.

## Roadmap failure coverage

| Failure | Primary task |
| --- | --- |
| Worker dies after claim | [03-03](03-lease-recovery/03-task.md) |
| Worker dies after effect before commit | [03-04](03-lease-recovery/04-task.md), [release demo](04-retry/06-task.md) |
| Stale worker reports after a new claim | [03-04](03-lease-recovery/04-task.md), [08-03](08-api-idempotency/03-task.md) |
| Engine dies during wakeup processing | [01-06](01-durable-sequence/06-task.md), [01-12](01-durable-sequence/12-task.md) |
| Two schedulers recover a lease | [03-03](03-lease-recovery/03-task.md) |
| Temporary PostgreSQL outage | [01-12](01-durable-sequence/12-task.md), [04-06](04-retry/06-task.md) |
| Duplicate external event | [07-03](07-external-events/03-task.md), [07-05](07-external-events/05-task.md) |

Durable execution is at-least-once. Demonstration activities deduplicate external effects using stable task_run_id; the plan does not promise exactly-once external effects.
