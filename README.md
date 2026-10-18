# TaskManager

A distributed task orchestrator built with Go and PostgreSQL.

The executable supports database checks, migrations, and a single worker running
durable sequences. HTTP publication and start endpoints are a later task.

## Prerequisites

- [Go 1.27.1](https://go.dev/dl/) on `PATH`; `go.mod` and the Makefile select the
  verification toolchain through [GOTOOLCHAIN](https://go.dev/doc/toolchain).
- GNU Make 4.3 and a POSIX shell with `sed`.
- For integration tests: Python 3.10+ to launch the harness, Docker Compose v2 with a Linux
  daemon, and access to Docker Hub/PyPI. Tests themselves run on Python 3.13.12
  and PostgreSQL 18.3; image digests and Python dependencies are pinned in
  [docker-compose.test.yml](docker-compose.test.yml) and [requirements](tests/requirements.txt).

## Development commands

```sh
make fmt        # format Go
make check      # formatting, vet, build, Go unit tests
make coverage   # local Go race and coverage reports
make test-python # isolated PostgreSQL integration and failure suites
```

`make test-python` builds the Go application and test probe, then uses
[docker-compose.test.yml](docker-compose.test.yml) for PostgreSQL, the Python
runner, health checks and networking. Each invocation has a unique Compose project.
The launcher copies test artifacts with `compose cp` so remote daemons need no
checkout bind mount. The test runner waits for PostgreSQL health, and its exit
code determines the command result. `compose down --volumes` runs on success,
failure or interruption and removes only that invocation's project resources.

Authentication is generated per run and passed through process environments,
never command arguments or committed values. The launcher accepts no database URL,
publishes no ports, and each test uses its own disposable database. Missing
prerequisites and failed cleanup fail the command. After a forced kill, inspect
the specific `taskmanager-test-<uuid>` Compose project before cleaning it up.

## Database commands

Set `DATABASE_URL` to a PostgreSQL connection string with the desired TLS policy.
`DATABASE_TIMEOUT` bounds connection/migration work and each worker database operation
(default `10s`, maximum `1m`); it does not limit the worker process lifetime.
The pgx `database/sql` driver owns connection parsing and pooling. Supply runtime
configuration through your environment or secret manager; never commit it.
Driver errors are redacted by the CLI.

```sh
make build
./bin/orchestrator db-check
./bin/orchestrator migrate
```

Choose the TLS policy in your runtime connection configuration. Embedded Goose
migrations use `NNNN_name.sql` and `-- +goose Up` / `-- +goose Down` sections.
Goose records versions in `goose_db_version`, holds a PostgreSQL session advisory
lock and commits each migration independently. A failing migration rolls back;
previously completed versions remain applied. Applied files must not be edited;
Goose tracks versions rather than content checksums. Do not use nontransactional
migrations or explicit transaction control. Domain tables live in the `orchestrator` schema.
Migration cleanup has a separate one-second timeout after command cancellation.

## Run a worker

After applying migrations, start one process:

```sh
./bin/orchestrator run
```

The command polls every 100 ms and claims one task at a time with a two-minute
lease. Registered demo task keys `A`, `B`, `C` each increment `n` and append their
stable task ID to `keys`: a sequence started with `{"n":1}` produces `n: 4` and
three keys. Definitions and runs can currently be created through the Go services;
the CLI does not publish or start them. These pure demos make no external effects.

Activities run after claim commit and must honor cancellation. Their local budget
includes claim latency and reserves `DATABASE_TIMEOUT` for completion; PostgreSQL
still decides lease validity. Unknown activities and execution errors fail the attempt.
Persistence errors stop the process without retrying uncertain commits. SIGINT/SIGTERM
cancels polling and in-flight work. Interrupted RUNNING attempts remain pending recovery
until stage 3; restarting alone does not recover them.

## Test conventions and CI

[CI](.github/workflows/ci.yml) builds, checks formatting/static analysis and runs
Go race tests plus both Python suites. Coverage percentages remain in logs;
`make coverage` produces optional local reports.

Configure fake time before constructing time-dependent components. The shared
Python `clock` fixture fixes application time and replaces the database-time
function only inside its isolated test database. Advance it explicitly; each
scenario starts fresh. Production uses `clock_timestamp()` through
`orchestrator.database_now()` and exposes no clock override flag or environment
variable. Worker unit tests use a shared manual clock with explicit advancement. Startup
polling and process timeouts bound infrastructure; they do not prove business
outcomes. See the [test-support contract](ai/references/02-code-structure.md#time-and-reusable-test-support).

## Project documentation

- [Project references](ai/references/README.md)
- [Implementation tasks](ai/tasks/README.md)
- [Contributor and agent rules](AGENTS.md)
