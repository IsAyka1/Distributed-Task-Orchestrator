# 08-01. Define idempotency keys and persist start requests

- Dependency: [07-05](../07-external-events/05-task.md) must meet its acceptance criteria.
- PR size: estimated **100–230 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `codex/<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Define key scope, fingerprinting, retention, and payload conflicts; add start-idempotency storage and constraints. Leave StartWorkflow/HTTP wiring to 08-02.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [ ] Distinguish no key, identical repeated request, and same key with different content. Fingerprints are deterministic. Retention prevents premature key reuse.
- [ ] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [ ] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [ ] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [ ] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [ ] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Go canonicalization tests; Python integration: unique keys per namespace, independent namespaces, migration, and fingerprint persistence. Configure fake time before retention checks.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
