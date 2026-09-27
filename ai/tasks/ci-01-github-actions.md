# CI-01. Add GitHub Actions verification

Status: Done

PR: https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/5

- Dependency: [00-02](00-domain/02-task.md) bootstrap commands are complete.
- Scope: automate build, Go style, Go tests, Python test gating, and Go coverage.
- Python harness implementation remains in [01-01](01-durable-sequence/01-task.md).
- Business-logic changes: 0; CI/build scaffolding and documentation only.

## Definition of Done

- [x] CI runs on pull requests and pushes to `main`, with manual dispatch available.
- [x] Build, `gofmt`, `go vet`, and Go tests with race detection pass.
- [x] Go coverage includes all packages and produces a summary and downloadable reports.
- [x] Python tests explicitly skip while absent; existing tests require the harness and propagate failures.
- [x] Workflow syntax, Python detection branches, documentation, and diff are reviewed.
- [x] Changes are committed and pushed on a dedicated branch with an open PR.

Verification: `make check coverage`, `actionlint .github/workflows/ci.yml`, focused
Python-gate checks, and `git diff --check` passed. Go statement coverage: 100.0%.
[Hosted CI](https://github.com/IsAyka1/Distributed-Task-Orchestrator/actions/runs/36346008403)
passed and uploaded coverage reports; Python integration/failure tests were
explicitly skipped because no harness/tests exist. Full verification details are
in the PR description.
