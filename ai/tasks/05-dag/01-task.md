# 05-01. Define DAG execution semantics

- Dependency: [04-06](../04-retry/06-task.md) must meet its acceptance criteria.
- PR size: estimated **0 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `codex/<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Record fail-fast behavior, treatment of running branches, input/output merging, and unreachable-task outcomes. Reuse definition cycle validation; enable DAG starts with the evaluator in 05-02.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [ ] The contract covers diamonds, multiple roots, and branch failure without ambiguous outcomes. DAG admission remains tied to evaluator support.
- [ ] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [ ] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [ ] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [ ] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [ ] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Review diamond/disconnected/failing-predecessor examples. Go validation regression tests where applicable. Resolve product choices before 05-02.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
