# 01-01. Add PostgreSQL configuration and a Python integration harness

Status: Done

PR: https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/8

- Dependency: [00-05](../00-domain/05-task.md) must meet its acceptance criteria.
- PR size: estimated **80–200 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Add connection configuration through the pgx driver, Goose migrations, isolated
test database setup, and documented Python integration commands. Generate test
authentication at runtime; commit no credential values. Share fixtures, process helpers, reusable mocks, and a controlled clock seam.

Expose `make test-python` for the existing CI Python gate: install pinned test
dependencies, provision and clean up isolated PostgreSQL, and run the integration
and failure suites. Align the workflow's Python selection with the harness pin. Missing prerequisites or failed tests must fail the command.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [x] One command prepares an isolated database. Missing configuration produces actionable errors. Cleanup touches only test resources. Fake application and database time can be configured before temporal scenarios; test controls are inaccessible in production.
- [x] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [x] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [x] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [x] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [x] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Python tests: setup/cleanup twice, reachable/unreachable database, and clock reset between tests. Go unit checks must not disguise missing integration prerequisites as successful coverage.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.
