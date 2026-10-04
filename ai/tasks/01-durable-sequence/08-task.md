# 01-08. Accept results from valid attempts atomically

- Status: Done
- PR: [#16](https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/16)

- Dependency: [01-07](../01-durable-sequence/07-task.md) must meet its acceptance criteria.
- PR size: estimated **160–300 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Contract: [MVP decision 0001](../../../docs/decisions/0001-mvp-contract.md).

CompleteAttempt validates token, owner, RUNNING, and unexpired lease; closes the attempt, updates the task, and enqueues a wakeup.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [x] Success and final error persist together with the wakeup. Invalid or expired reports do not mutate data. The stage-1 attempt limit is one.
- [x] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [x] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [x] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [x] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [x] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Python integration tests: wrong token/owner, expiry, rollback between task and wakeup, success/error, and duplicate rejection. Initialize mocked time before claims and explicitly advance to expiry.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
