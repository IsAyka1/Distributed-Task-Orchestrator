# Stage 3. Lease/recovery

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [02-03](../02-multi-worker/03-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [03-01](01-task.md) | Renew leases through fenced heartbeats | 150–300 |
| [03-02](02-task.md) | Recover expired leases atomically | 160–310 |
| [03-03](03-task.md) | Run recovery scheduling and verify competing schedulers | 170–320 |
| [03-04](04-task.md) | Verify fencing after an external effect and crash | 0 |

## Stage acceptance

Work recovers after a worker crash, and stale attempts cannot persist results.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
