# Stage 6. Timers/timeouts

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [05-03](../05-dag/03-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [06-01](01-task.md) | Define timer and deadline contracts | 0 |
| [06-02](02-task.md) | Persist timers and fire them at due_at | 180–340 |
| [06-03](03-task.md) | Apply task and attempt timeouts | 170–330 |
| [06-04](04-task.md) | Enforce workflow deadlines | 150–310 |
| [06-05](05-task.md) | Verify timer restarts and time-boundary races | 0 |

## Stage acceptance

Waiting and deadlines are durable; restart does not reset time or terminal outcomes.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
