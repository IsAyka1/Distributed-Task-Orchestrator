# 02-01. Extend claim to bounded batches

Status: Done

PR: [#21](https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/21)

- Dependency: [01-12](../01-durable-sequence/12-task.md) must meet its acceptance criteria.
- PR size: estimated **100–220 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Add a limit parameter, stable available_at/id ordering, and batch-size validation to the single claim implementation.

Use an atomic batch with limits 1–100 and one eligible task per workflow while execution
is sequential. Acquire all workflow locks before wakeup/task locks; short batches under
contention are valid. Keep single-task callers on the same implementation with limit 1.
The [MVP lock contract](../../../docs/decisions/0001-mvp-contract.md#transactions-leases-and-lock-order)
records this bounded exception to the stage-1 one-workflow transaction rule.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [x] Each claimed task gets its own attempt. Empty queues are valid. Invalid limits are rejected and transactions remain short.
- [x] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [x] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [x] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [x] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [x] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Python/PostgreSQL tests: limits 1/N, future tasks, empty queue, batch rollback, and skipping a row locked by another connection. Mock time before setup.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
