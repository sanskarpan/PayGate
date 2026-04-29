# PayGate — Documentation Index

> A production-grade, multi-tenant payment platform. Built for senior engineer portfolios.

---

## Documentation suite

| Document | Purpose |
|----------|---------|
| [PRD.md](./docs/PRD.md) | Product requirements, user roles, scope, domain entities, core flows, NFRs |
| [SPEC.md](./docs/SPEC.md) | Technical specification: state machines, API contracts, ledger design, outbox pattern, idempotency, webhook engine, settlement engine, security |
| [ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Service map, responsibilities, infrastructure topology, technology decisions, failure domains, deployment strategy |
| [DATA-FLOW.md](./docs/DATA-FLOW.md) | End-to-end data flows for every operation: payment lifecycle, event propagation, refunds, settlements, reconciliation, idempotency, auto-capture, webhook retries |
| [DATABASE.md](./DATABASE.md) | Complete PostgreSQL schema: all tables, indexes, constraints, migration strategy |
| [API-CONTRACTS.md](./docs/API-CONTRACTS.md) | Full API reference: request/response shapes, headers, error format, webhook event catalog |
| [TESTING-STRATEGY.md](./docs/TESTING-STRATEGY.md) | Testing pyramid: unit tests, integration tests, contract tests, E2E tests, chaos tests, load tests, CI pipeline |
| [FAILURE-MODES.md](./docs/FAILURE-MODES.md) | Every failure the system can encounter, what happens, and how it recovers |
| [CHECKLIST.md](./CHECKLIST.md) | Phase-by-phase implementation checklist with concrete deliverables |
| [PROMPT.md](./PROMPT.md) | Claude Code system prompt and task sequence for building the project |
| [RUNBOOK.md](./docs/RUNBOOK.md) | Operational procedures, incident playbooks, monitoring dashboards, backup/recovery |
| [FOUNDATION-REVIEW.md](./docs/FOUNDATION-REVIEW.md) | Direct assessment of what is solid, what was assumption-heavy, and what must be proven |

---

## Foundation assessment

This is a strong conceptual foundation, not yet a complete engineering foundation. The docs correctly identify the hard parts of payments: explicit state machines, double-entry ledgering, idempotency, transactional event publishing, settlements, reconciliation, and operational recovery. Those are the right primitives.

The weak point is that several sections previously assumed distributed consistency would "just work" across services. That is dangerous in a payments system. The implementation must choose one consistency boundary for money-critical operations:

- **Recommended for this project**: implement Phase 1 as a modular monolith in Go with strict package boundaries and one PostgreSQL transaction for payment state, ledger entries, audit event, and outbox event.
- **Extraction path**: keep service-shaped packages and APIs so services can be split later, after the invariants are proven with tests.
- **Do not do initially**: make Payment, Ledger, Settlement, and Outbox independent network services on the synchronous money path without a saga/command protocol. That creates dual-write and orphan-ledger edge cases.

If built this way, the project is credible. If built as nine loosely coordinated microservices from day one, it is mostly architectural theatre.

---

## Quick start

1. Read **docs/PRD.md** to understand what you're building and why
2. Read **docs/ARCHITECTURE.md** to understand the system shape
3. Open **CHECKLIST.md** and start with Phase 0 (project setup)
4. Use **PROMPT.md** as your Claude Code context when implementing
5. Reference **docs/SPEC.md**, **DATABASE.md**, and **docs/API-CONTRACTS.md** as you build each service
6. Use **docs/TESTING-STRATEGY.md** to write tests alongside code
7. Use **docs/FAILURE-MODES.md** to validate your error handling
8. Use **docs/RUNBOOK.md** to build your operational dashboards

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

---

## Advanced extension track

If you want to push this beyond a strong portfolio backend into a systems-design-heavy build, implement the advanced distributed track consistently across docs:

- Service extraction with saga orchestration and idempotent command handlers
- Event schema registry, compatibility checks, and consumer contract gates in CI
- Ledger reservations/holds, release/commit flows, and payout rail simulation
- Exactly-once illusions handled explicitly via at-least-once + dedup strategy
- Multi-region readiness patterns: DR runbook, failover drills, reconciliation catch-up
- Risk model evolution: rule engine + supervised scoring + explainability trail
