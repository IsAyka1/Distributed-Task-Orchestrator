# 00-04. Implement task and attempt transitions

Status: Done

PR: [#6](https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/6)

- Dependency: [00-03](../00-domain/03-task.md) must meet its acceptance criteria.
- PR size: estimated **120–250 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` without `codex` in the name. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Contract: [MVP decision 0001](../../../docs/decisions/0001-mvp-contract.md).

Add pure PENDING/READY/RUNNING/SUCCEEDED/FAILED transitions and baseline attempt outcomes, with events separate from I/O.

Use a value snapshot with a one-attempt policy, counters and the latest attempt
outcome. Keep IDs, timestamps, payload propagation, workflow/dependency guards,
lease fencing and persistence in their designated engine/application/storage
tasks; the pure transition returns a proposal, not authorization to execute.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [x] Invalid transitions return errors without mutation. SUCCEEDED and final FAILED tasks cannot restart. Attempt outcomes remain consistent with logical-task outcomes.
- [x] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [x] Use reusable mocks and controlled time if needed. Pure transitions require neither; report Python integration tests as skipped until the stage-1 harness exists.
- [x] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [x] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [x] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Go transition tables, repeated completion, and unchanged objects after rejection. Configure any fake clock before invoking transitions.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
