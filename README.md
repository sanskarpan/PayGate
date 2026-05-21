# PayGate

Production-grade, multi-tenant payment platform with explicit state machines, double-entry ledgering, transactional outbox, schema governance, saga orchestration, payout rail simulation, and an operator dashboard.

---

## Quick start

```bash
docker compose up -d
go run ./cmd/api-gateway
cd dashboard && pnpm install && pnpm dev
```

Optional extracted-worker mode:

```bash
SAGA_WORKER_ENABLED=false go run ./cmd/api-gateway &
go run ./cmd/saga-orchestrator
```

## Demo flow

1. Create a merchant and API key.
2. Create an order.
3. Authorize and capture a payment.
4. Register a webhook.
5. Process refund, settlement, payout, and dispute flows.
6. Inspect saga dispatches, dead letters, payout timelines, reconciliation, and observability screens.

## Screenshots

Login

![Dashboard login](./docs/screenshots/dashboard-login.png)

Orders

![Orders list](./docs/screenshots/dashboard-orders.png)

Webhooks

![Webhook operations](./docs/screenshots/dashboard-webhooks.png)

Settlements

![Settlement operations](./docs/screenshots/dashboard-settlements.png)

Observability

![Observability dashboard](./docs/screenshots/dashboard-observability.png)

## Demo instructions

1. Start infrastructure and apply migrations with `bash scripts/dr/recreate_local_env.sh`.
2. Run the API with `DATABASE_URL=postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable REDIS_ADDR=localhost:6380 KAFKA_BROKERS=localhost:9092 go run ./cmd/api-gateway`.
3. Run the dashboard with `cd dashboard && pnpm install && pnpm dev`.
4. Create a merchant: `POST /v1/merchants`.
5. Bootstrap the first admin API key: `POST /v1/merchants/{merchant_id}/keys`.
6. Bootstrap the dashboard user: `POST /v1/merchants/{merchant_id}/users/bootstrap`.
7. Log into `http://localhost:3001`, then create orders, authorize/capture payments, run settlements, initiate payouts, and inspect disputes, webhooks, reconciliation, and observability pages.

## Verification

Backend and browser verification now have first-class commands:

```bash
go test ./...
go test -tags=integration ./tests/integration/...
./scripts/test/run_full_verification.sh
cd dashboard && pnpm lint && pnpm build && pnpm test:e2e
```

The Playwright suite seeds a real merchant, logs in through the dashboard, exercises the money flow through live APIs, and verifies webhook delivery through a local receiver route.

For heavier local verification:

```bash
START_API=true ./scripts/test/run_load_smoke.sh
START_API=true ./scripts/test/run_chaos_suite.sh
```

## Docs

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
| [openapi.yaml](./docs/openapi.yaml) | OpenAPI 3.0 reference for major endpoints |
| [WEBHOOK-EVENT-CATALOG.md](./docs/WEBHOOK-EVENT-CATALOG.md) | Event subjects mapped to versioned JSON schema fixtures |
| [INTEGRATION-GUIDE.md](./docs/INTEGRATION-GUIDE.md) | Merchant integration guidance |
| [DEPLOYMENT-GUIDE.md](./docs/DEPLOYMENT-GUIDE.md) | Local and production deployment flow |
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

## Operations and DR tooling

- [`scripts/dr/verify_postgres_backup.sh`](./scripts/dr/verify_postgres_backup.sh): dump, restore, and verify a Postgres backup
- [`scripts/dr/recreate_local_env.sh`](./scripts/dr/recreate_local_env.sh): recreate the local infrastructure stack and reapply migrations

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
