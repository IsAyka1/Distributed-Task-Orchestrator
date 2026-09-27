# 01-06. Apply engine evaluation through durable wakeups

- Dependency: [01-05](../01-durable-sequence/05-task.md) must meet its acceptance criteria.
- PR size: estimated **180–330 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `codex/<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Contract: [MVP decision 0001](../../../docs/decisions/0001-mvp-contract.md).

EvaluateWorkflow locks records in the agreed order, applies pure evaluation, and removes the wakeup atomically.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [ ] The first step activates; success advances the chain; terminal outcomes persist. A concurrent task update cannot lose its reevaluation signal.
- [ ] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [ ] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [ ] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [ ] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [ ] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Python/PostgreSQL tests: rollback before wakeup deletion, competing evaluations, and task-update/wakeup/evaluation races synchronized with reusable barriers.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
