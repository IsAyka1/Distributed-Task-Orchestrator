# Stage 4. Retry

[All tasks](../README.md) · [Roadmap](../../references/04-roadmap.md)

The preceding stage, including [03-04](../03-lease-recovery/04-task.md), must meet its acceptance criteria.

Follow the order below; each task depends on the previous one. One task maps to one PR, limited to 500 changed business-logic lines. Tests and documentation are excluded from the count. A zero estimate identifies work without business-logic changes, such as documentation or test scenarios.

| Task | PR outcome | Estimated business-logic lines |
| --- | --- | --- |
| [04-01](01-task.md) | Define retry policy and classify errors | 100–220 |
| [04-02](02-task.md) | Calculate backoff and RETRY_WAIT transitions | 110–240 |
| [04-03](03-task.md) | Persist retry decisions during completion and recovery | 160–300 |
| [04-04](04-task.md) | Activate retries when their due time arrives | 100–220 |
| [04-05](05-task.md) | Expose attempt history through the API | 120–260 |
| [04-06](06-task.md) | Demonstrate the first useful release | 0 |

## Stage acceptance

Bounded retries preserve complete history and end in success or FAILED.

Verify every task's DoD and the working stage scenario. Write integration checks in Python; configure reusable mocks and fake time before execution. Contract/data changes always update the affected references and `AGENTS.md`. Apply the [shared execution rules](../README.md#execution-rules).
