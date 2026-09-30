# 01-03a. Keep status enums in Go

Status: Open

PR: https://github.com/IsAyka1/Distributed-Task-Orchestrator/pull/11

Dependency: [01-03](03-task.md). Follow the [shared execution rules](../README.md#execution-rules).

## Outcome

Keep enum definitions and validation in Go. Remove status allowed-value checks
from initial migrations 0002/0003 and remove SQL enum tests and unused test helpers.
This explicitly requested baseline rewrite is an exception to migration immutability;
it does not upgrade databases that already applied the old migrations.

## Definition of Done

- [x] Project rules and data reference describe the enum boundary.
- [x] Fresh databases use plain text status columns without value lists; other invariants remain enforced.
- [ ] Go and isolated Python checks pass.
- [x] Diff reviewed, business-logic limit verified, changes committed and pushed, and PR opened.
