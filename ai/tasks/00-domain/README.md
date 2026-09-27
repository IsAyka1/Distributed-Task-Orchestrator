# Stage 0. Domain

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

Begin with the MVP contract task below.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [00-01](01-task.md) | Define the MVP contract and resolve model choices | 0 |
| [00-02](02-task.md) | Bootstrap the Go module and verification commands | 0–80 |
| [00-03](03-task.md) | Add immutable definitions and graph validation | 150–280 |
| [00-04](04-task.md) | Implement task and attempt transitions | 120–250 |
| [00-05](05-task.md) | Evaluate sequential workflows with pure logic | 150–280 |

## Stage acceptance

Pure domain logic validates definitions, transitions, and sequential workflow outcomes.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
