# Stage 8. API idempotency

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [07-05](../07-external-events/05-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [08-01](01-task.md) | Define idempotency keys and persist start requests | 100–230 |
| [08-02](02-task.md) | Make workflow start and its HTTP endpoint idempotent | 160–310 |
| [08-03](03-task.md) | Deduplicate accepted attempt reports | 140–290 |

## Stage acceptance

Repeated start requests return the original run, and repeated reports cannot cause additional transitions.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
