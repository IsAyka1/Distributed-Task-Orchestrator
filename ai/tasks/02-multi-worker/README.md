# Stage 2. Multi-worker

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [01-12](../01-durable-sequence/12-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [02-01](01-task.md) | Extend claim to bounded batches | 100–220 |
| [02-02](02-task.md) | Run multiple worker processes with backpressure | 120–260 |
| [02-03](03-task.md) | Prove concurrent claim behavior on PostgreSQL | 0 |

## Stage acceptance

Multiple processes share the queue without two valid attempts for one task.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
