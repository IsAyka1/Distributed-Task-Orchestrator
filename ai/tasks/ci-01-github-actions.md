# CI-01. Add GitHub Actions verification

Status: In progress

- Dependency: [00-02](00-domain/02-task.md) bootstrap commands are complete.
- Scope: automate build, Go style, Go tests, Python test gating, and Go coverage.
- Python harness implementation remains in [01-01](01-durable-sequence/01-task.md).
- Business-logic changes: 0; CI/build scaffolding and documentation only.

## Definition of Done

- [ ] CI runs on pull requests and pushes to `main`, with manual dispatch available.
- [ ] Build, `gofmt`, `go vet`, and Go tests with race detection pass.
- [ ] Go coverage includes all packages and produces a summary and downloadable reports.
- [ ] Python tests explicitly skip while absent; existing tests require the harness and propagate failures.
- [ ] Workflow syntax, Python detection branches, documentation, and diff are reviewed.
- [ ] Changes are committed and pushed on a dedicated branch with an open PR.

Verification commands and results are recorded in the PR description.
