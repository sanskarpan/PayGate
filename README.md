# PayGate — Documentation Index

> A production-grade, multi-tenant payment platform. Built for senior engineer portfolios.

---

## Documentation suite

| Document | Purpose |
|----------|---------|
| [PRD.md](./PRD.md) | Product requirements, user roles, scope, domain entities, core flows, NFRs |
| [SPEC.md](./SPEC.md) | Technical specification: state machines, API contracts, ledger design, outbox pattern, idempotency, webhook engine, settlement engine, security |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Service map, responsibilities, infrastructure topology, technology decisions, failure domains, deployment strategy |
| [DATA-FLOW.md](./DATA-FLOW.md) | End-to-end data flows for every operation: payment lifecycle, event propagation, refunds, settlements, reconciliation, idempotency, auto-capture, webhook retries |
| [DATABASE.md](./DATABASE.md) | Complete PostgreSQL schema: all tables, indexes, constraints, migration strategy |
| [API-CONTRACTS.md](./API-CONTRACTS.md) | Full API reference: request/response shapes, headers, error format, webhook event catalog |
| [TESTING-STRATEGY.md](./TESTING-STRATEGY.md) | Testing pyramid: unit tests, integration tests, contract tests, E2E tests, chaos tests, load tests, CI pipeline |
| [FAILURE-MODES.md](./FAILURE-MODES.md) | Every failure the system can encounter, what happens, and how it recovers |
| [CHECKLIST.md](./CHECKLIST.md) | Phase-by-phase implementation checklist with concrete deliverables |
| [PROMPT.md](./PROMPT.md) | Claude Code system prompt and task sequence for building the project |
| [RUNBOOK.md](./RUNBOOK.md) | Operational procedures, incident playbooks, monitoring dashboards, backup/recovery |

---

## Quick start

1. Read **PRD.md** to understand what you're building and why
2. Read **ARCHITECTURE.md** to understand the system shape
3. Open **CHECKLIST.md** and start with Phase 0 (project setup)
4. Use **PROMPT.md** as your Claude Code context when implementing
5. Reference **SPEC.md**, **DATABASE.md**, and **API-CONTRACTS.md** as you build each service
6. Use **TESTING-STRATEGY.md** to write tests alongside code
7. Use **FAILURE-MODES.md** to validate your error handling
8. Use **RUNBOOK.md** to build your operational dashboards

---

## What makes this a senior-level project

This is not a CRUD application with a "pay" button. The documentation suite covers:

- **State machines** with explicit transition tables and invalid-transition rejection
- **Double-entry ledger** with journal entry specs for every monetary flow
- **Transactional outbox** to guarantee event delivery without dual-write problems
- **Idempotency** with three-case handling (new, completed, in-progress)
- **Webhook delivery engine** with exponential backoff, dead-letter queues, and replay
- **Three-way reconciliation** (payment ↔ ledger ↔ settlement) with mismatch classification
- **Failure mode catalog** with 15+ documented failure scenarios and recovery paths
- **Testing strategy** that covers chaos testing and load testing, not just unit tests
- **Operational runbook** with incident playbooks for every P1/P2 scenario

Each document is designed to be directly implementable — no hand-waving, no "left as an exercise."
