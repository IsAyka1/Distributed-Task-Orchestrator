# 01-02. Create definition and workflow-run schema

Status: In progress

- Dependency: [01-01](../01-durable-sequence/01-task.md) must meet its acceptance criteria.
- PR size: estimated **100–200 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Add workflow_definitions and workflow_runs migrations with FKs, immutable version identity, and agreed unique/check constraints.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [ ] Each run references one definition version. Publishing v2 leaves v1 unchanged. Constraints follow 00-01.
- [ ] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [ ] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [ ] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [ ] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [ ] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Python/PostgreSQL checks: empty database migration, repeat migrator run, FK/unique/status violations, and JSON round-trip.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
