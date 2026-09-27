# Stage 9. Observability

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [08-03](../08-api-idempotency/03-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [09-01](01-task.md) | Log transitions with structured correlation fields | 120–260 |
| [09-02](02-task.md) | Measure queues, leases, and retries | 140–290 |
| [09-03](03-task.md) | Trace HTTP requests, workflows, and attempts | 140–300 |
| [09-04](04-task.md) | Document diagnosis and verify operational signals | 0 |

## Stage acceptance

Telemetry and history explain every retry and reason for missing progress.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
