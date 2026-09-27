# Stage 7. External events

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [06-05](../06-timers-timeouts/05-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [07-01](01-task.md) | Define event waiting and delivery contracts | 0 |
| [07-02](02-task.md) | Persist WAITING and correlation | 140–280 |
| [07-03](03-task.md) | Persist and deduplicate incoming events | 140–280 |
| [07-04](04-task.md) | Consume events and complete waits atomically | 160–310 |
| [07-05](05-task.md) | Expose event ingestion over HTTP and verify restart | 130–270 |

## Stage acceptance

WAITING persists, events correlate correctly, and duplicate delivery produces one stored continuation.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
