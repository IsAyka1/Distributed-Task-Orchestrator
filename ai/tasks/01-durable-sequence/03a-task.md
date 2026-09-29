# 01-03a. Keep status enums in Go

Status: Done

PR: https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/11

Dependency: [01-03](03-task.md). Follow the [shared execution rules](../README.md#execution-rules).

## Outcome

Keep enum definitions and validation in Go. Remove status allowed-value checks
through a forward migration; preserve existing rows, lease consistency, foreign
keys, uniqueness and attempt budgets. Applied migrations remain immutable.

## Definition of Done

- [x] Project rules and data reference describe the enum boundary.
- [x] Fresh and existing databases use plain text status columns without value lists; other invariants remain enforced.
- [x] Go and isolated Python checks pass, including data preservation during upgrade.
- [x] Diff reviewed, business-logic limit verified, changes committed and pushed, and PR opened.
