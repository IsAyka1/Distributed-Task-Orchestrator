# TaskManager

A distributed task orchestrator built with Go and PostgreSQL.

The current executable is a bootstrap entry point: it exits successfully without
starting services. Workflow execution and database support follow in later tasks.

## Prerequisites

- [Go 1.27.1](https://go.dev/dl/) on `PATH`; `go.mod` pins the verification
  toolchain. The Makefile selects that exact version through
  [GOTOOLCHAIN](https://go.dev/doc/toolchain). An older Go launcher may download it
  on first use; install it beforehand for offline checks.
- GNU Make 4.3 and a POSIX shell with `sed` for the convenience commands below.
  Formatting and testing use Go's bundled tools; there are no third-party Go dependencies.
- PostgreSQL and Python are not required for this bootstrap. Their versions and
  integration commands will be pinned with the harness in
  [task 01-01](ai/tasks/01-durable-sequence/01-task.md).

## Development commands

Run from the repository root:

```sh
make fmt        # apply gofmt
make check-fmt  # fail on unformatted Go files without changing them
make vet        # run Go static analysis
make build      # build bin/orchestrator
make test       # run all Go unit tests
make check      # formatting, static analysis, build, and unit tests
make coverage   # run all Go tests with race detection and coverage reports
./bin/orchestrator
```

`go test` runs the pure definition validation and immutability tests in
`internal/workflow`; the bootstrap command has no unit test cases.
With Go 1.27.1 installed, the task's direct verification commands also work:

```sh
go build ./cmd/orchestrator
go test ./...
gofmt -l .      # must print nothing
```

## Continuous integration

[CI](.github/workflows/ci.yml) builds, checks Go style, and runs tests with race
detection on pull requests and pushes to `main`. Run `make coverage` locally
for coverage reports.

## Test conventions

Before constructing any time-dependent component, create a fresh reusable fake
clock with a fixed UTC instant and explicitly configured timers/tickers. Inject it
into the component, advance it explicitly, and use deterministic jitter. Pure
functions may take an explicit instant. Never use real sleeps to prove behavior.
Introduce clock interfaces and shared fakes with the first actual consumer; use
named, typed fixtures with per-test setup/reset rather than duplicated test doubles.

Python integration and failure tests will use real isolated PostgreSQL, shared
fixtures, and barriers for races. Configure application and database time together
before temporal scenarios and reset both between tests. Production lease decisions
use database time; test clock controls must remain inaccessible in production.
See the [test-support contract](ai/references/02-code-structure.md#time-and-reusable-test-support).

## Project documentation

- [Project references](ai/references/README.md)
- [Implementation tasks](ai/tasks/README.md)
- [Contributor and agent rules](AGENTS.md)
