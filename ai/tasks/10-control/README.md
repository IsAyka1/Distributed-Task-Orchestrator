# Stage 10. Control

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [09-04](../09-observability/04-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [10-01](01-task.md) | Define cancellation and pause/resume contracts | 0 |
| [10-02](02-task.md) | Persist cancellation and prevent new work | 160–320 |
| [10-03](03-task.md) | Stop running activities cooperatively | 140–290 |
| [10-04](04-task.md) | Persist pause and enforce scheduling rules | 180–350 |
| [10-05](05-task.md) | Resume paused workflows and verify the control cycle | 160–330 |

## Stage acceptance

Cancellation and pause/resume persist across restarts and cannot revive terminal workflows.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
