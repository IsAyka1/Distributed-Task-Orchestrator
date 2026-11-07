# Durable sequence restart demonstration

Run `make test-python` from the repository root. The isolated harness builds the
Go application and probe, runs the integration/failure suites, and removes its
Compose resources. No existing database or credentials are needed; see the
[development prerequisites](../../README.md#prerequisites).

The [stage-1 scenarios](../../tests/failure/test_sequence_restart.py) exercise the
real worker, application services, PostgreSQL transactions and HTTP inspection:

| Scenario | Observable result |
| --- | --- |
| Exit after A commits, then start a new worker process | Only B/C execute; A's task and attempt records remain unchanged. |
| Kill the engine process at a SQL barrier before evaluation commits | B's provisional activation rolls back and the wakeup survives. |
| Reject evaluation at deferred commit | The process fails without a success result; durable progress and the wakeup remain unchanged. |
| Refuse connections and terminate clients of the isolated database during evaluation | Uncommitted changes roll back, GET returns 503, and a new worker continues after access is restored. |

Each recovered sequence ends with three successful attempts, no active tokens or
wakeup, and GET output `n: 4` with the three stable task IDs. Another process
invocation performs no work and leaves the terminal snapshot unchanged.

Application and database clocks are fixed before each scenario. SQL barriers
select the failure point; wall-clock timeouts only detect a hung test. The outage
helper preserves one observer connection to inspect rollback and restores database
access in `finally`; it does not simulate a PostgreSQL host restart.

These scenarios restart **between attempts**. Recovery of interrupted RUNNING
attempts belongs to [stage 3](../../ai/tasks/03-lease-recovery/README.md).
