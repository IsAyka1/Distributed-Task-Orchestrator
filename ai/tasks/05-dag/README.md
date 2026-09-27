# Stage 5. DAG

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [04-06](../04-retry/06-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [05-01](01-task.md) | Define DAG execution semantics | 0 |
| [05-02](02-task.md) | Evaluate parallel branches and joins | 160–320 |
| [05-03](03-task.md) | Verify durable DAG execution under races and restarts | 0 |

## Stage acceptance

Independent branches execute concurrently, and joins wait for every mandatory dependency.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
