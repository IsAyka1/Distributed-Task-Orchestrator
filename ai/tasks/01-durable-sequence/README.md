# Stage 1. Durable sequence

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [00-05](../00-domain/05-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [01-01](01-task.md) | Add PostgreSQL configuration and a Python integration harness | 80–200 |
| [01-02](02-task.md) | Create definition and workflow-run schema | 100–200 |
| [01-03](03-task.md) | Create tasks, attempts, and wakeup schema | 120–230 |
| [01-04](04-task.md) | Persist and read immutable definitions | 140–260 |
| [01-05](05-task.md) | Start workflows and materialize tasks atomically | 160–300 |
| [01-06](06-task.md) | Apply engine evaluation through durable wakeups | 180–330 |
| [01-07](07-task.md) | Claim tasks atomically with attempts and leases | 150–290 |
| [01-08](08-task.md) | Accept results from valid attempts atomically | 160–300 |
| [01-09](09-task.md) | Run one worker and the engine loop | 180–330 |
| [01-10](10-task.md) | Expose definition publication and workflow start over HTTP | 150–290 |
| [01-11](11-task.md) | Expose workflow and task inspection over HTTP | 130–250 |
| [01-12](12-task.md) | Verify sequence restarts and temporary database outages | 0 |

## Stage acceptance

A single process executes A → B → C through PostgreSQL and resumes from committed progress after restart.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
