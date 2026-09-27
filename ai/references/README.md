# Project references

These documents describe the orchestrator's requirements, architecture, and data contracts.

- [Project context](01-context.md): goals, guarantees, and invariants.
- [Code structure](02-code-structure.md): package boundaries, transactions, and test architecture.
- [Data model](03-data-model.md): entities, transitions, leases, and wakeups.
- [Roadmap](04-roadmap.md): stage scope and failure criteria.

Use the [task index](../tasks/README.md) for PR-sized work and dependencies. Follow [AGENTS.md](../../AGENTS.md) for coding, testing, and review rules.

Whenever a contract or data model changes, update the affected references and `AGENTS.md` in the same PR. Keep requirements and design decisions here; keep task descriptions under `ai/tasks`. Do not record implementation-progress snapshots in either collection.
