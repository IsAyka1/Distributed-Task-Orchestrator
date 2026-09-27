# 00-02. Bootstrap the Go module and verification commands

Status: Done

PR: https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/3

- Dependency: [00-01](../00-domain/01-task.md) must meet its acceptance criteria.
- PR size: estimated **0–80 business-logic lines**; hard limit **500 additions + deletions of business logic**. Tests and documentation are excluded.
- Sources: [roadmap](../../references/04-roadmap.md), [context](../../references/01-context.md), [code structure](../../references/02-code-structure.md), [data model](../../references/03-data-model.md).
- Shared requirements: [execution rules](../README.md#execution-rules).

## Before starting

Create a new dedicated branch from the appropriate base, using `<task-id>-<short-description>` by default. Preserve unrelated changes; do not reuse another task's branch.

## Outcome and PR scope

Add go.mod, a minimal cmd/orchestrator, and repeatable formatting, build, and unit-test commands. Pin compatible tool versions during implementation.

Deliver one independently verifiable PR for this outcome. Keep adjacent capabilities in their own tasks.

## Definition of Done

- [x] The binary builds; no empty speculative packages are added; README documents prerequisites and commands. Establish reusable fake clock conventions before time-dependent code.
- [x] The checks below pass and the PR records actual commands and results. Documentation-only changes have their examples and links checked.
- [x] Reusable mocks are used where test doubles are needed. Time is mocked before time-dependent code runs. Integration tests are written in Python against real PostgreSQL.
- [x] Contract or data changes update the affected references and `AGENTS.md` in the same PR; linked tasks remain consistent.
- [x] The diff is reviewed and changes no more than 500 business-logic lines, excluding tests and documentation.
- [x] Task changes are committed and pushed on the dedicated branch, and a pull request is open against the intended base with the task reference, verification results, and business-logic line count. The final response includes its URL.

## Testing

Run go build ./cmd/orchestrator and go test ./... in a clean environment; verify gofmt leaves no changes.

## Split or decision boundary

Resolve conflicting contracts or product choices before dependent implementation. If business logic exceeds 500 changed lines, split the task into independently tested PRs and update dependencies. Keep regression tests with their behavior change; tests and documentation do not consume the limit.

## Verification results

Verified with Go 1.27.1 and GNU Make 4.3:

- `go build ./cmd/orchestrator`, `go test ./...`, and `make check` passed.
  The bootstrap has no unit test cases yet; Go reports `[no test files]`.
- `./bin/orchestrator` exited successfully; `gofmt -l .` produced no output;
  `git diff --check` passed.
- Build, test, check, and executable smoke passed again in a temporary Git
  repository with a minimal environment, fresh caches, and `GOPROXY=off`.
- In the temporary copy, `make check-fmt` rejected deliberately unformatted Go
  without modifying it; `make fmt` repaired it and the format check passed.
- Python integration/failure and race checks are not applicable: no database or
  concurrent code is introduced. Shared time/mock conventions are documented in
  README; implementations follow with actual consumers and the 01-01 harness.
- No application contract or data model changes; no migration is needed.
- Diff reviewed: **0 business-logic additions + deletions** against main.
  Module metadata, Makefile, ignore rules, and the six-line entry point are
  non-business scaffolding; all other changes are documentation.
