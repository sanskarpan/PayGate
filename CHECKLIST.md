# PayGate — Implementation Checklist

> Track your progress phase by phase. Every checkbox maps to a concrete deliverable. Don't skip ahead.

---

## Phase 0 — Project setup

### Repository and tooling
- [x] Initialize Go module: `go mod init github.com/{you}/paygate`
- [x] Set up monorepo structure (see directory layout below)
- [x] Start as a modular monolith: one backend deployable, strict internal package boundaries, extract workers later
- [x] Configure `golangci-lint` with strict config
- [x] Set up `Makefile` with targets: `build`, `test`, `lint`, `migrate`, `dev`, `docker`
- [x] Create `docker-compose.yml` with Postgres, Kafka (KRaft), Redis, MinIO
- [x] Create `docker-compose.test.yml` for integration tests
- [x] Set up database migration tool (`golang-migrate`)
- [x] Create initial migration: merchant and API key tables
- [x] Write `KSUID` ID generator with prefix support
- [x] Set up structured JSON logger (zerolog or slog)
- [x] Set up OpenTelemetry tracing (basic, wire through HTTP/gRPC)
- [x] Create health check endpoints (`/healthz`, `/readyz`) as shared middleware
- [x] Initialize Next.js frontend in `dashboard/` directory
- [x] Set up CI pipeline (lint → unit test → build → integration test)

### Directory layout
```
paygate/
├── cmd/
│   ├── api-gateway/        main.go
│   ├── order-service/      main.go
│   ├── payment-service/    main.go
│   ├── refund-service/     main.go
│   ├── settlement-service/ main.go
│   ├── ledger-service/     main.go
│   ├── webhook-service/    main.go
│   ├── recon-worker/       main.go
│   ├── outbox-relay/       main.go
│   └── gateway-proxy/      main.go
├── internal/
│   ├── order/              domain, service, repository, handler
│   ├── payment/            domain, service, repository, handler
│   ├── refund/             ...
│   ├── settlement/         ...
│   ├── ledger/             ...
│   ├── webhook/            ...
│   ├── recon/              ...
│   ├── merchant/           ...
│   ├── gateway/            simulated gateway client
│   ├── outbox/             outbox writer and relay
│   ├── auth/               API key auth, RBAC
│   ├── idempotency/        idempotency middleware
│   ├── ratelimit/          rate limiter
│   ├── audit/              audit logger
│   └── common/             shared types, errors, middleware
├── migrations/             SQL migration files
├── proto/                  protobuf definitions (gRPC)
├── config/                 YAML config files
├── scripts/                helper scripts
├── tests/
│   ├── integration/        integration tests
│   ├── e2e/                end-to-end tests
│   └── load/               k6 load test scripts
├── dashboard/              Next.js frontend
├── docs/                   this documentation suite
├── deployments/
│   ├── docker/             Dockerfiles per service
│   └── k8s/                Kubernetes manifests
├── docker-compose.yml
├── docker-compose.test.yml
├── Makefile
└── go.mod
```

---

## Phase 1 — Core payments backbone

### Merchant and API keys
- [x] Merchant registration endpoint: `POST /v1/merchants`
- [x] Merchant model with settings (auto_capture, fee_rate, etc.)
- [x] API key generation: `POST /v1/merchants/{id}/keys`
- [x] API key authentication middleware (Basic auth, bcrypt verification)
- [x] API key scoping (read, write, admin)
- [x] API key revocation endpoint
- [x] Unit tests: auth middleware, key validation, scope checking

### Order service
- [x] Order domain model with state machine
- [x] `POST /v1/orders` — create order
- [x] `GET /v1/orders/{id}` — fetch order
- [x] `GET /v1/orders` — list orders (cursor pagination)
- [x] Order expiry: set `expires_at` on creation (default 30 min)
- [x] Order expiry sweeper (CronJob or ticker goroutine)
- [x] Outbox: write `order.created` event in same transaction
- [x] Unit tests: state machine transitions, validation, pagination
- [x] Integration test: create order → verify in DB

### Payment service
- [x] Payment attempt model
- [x] Payment domain model with state machine
- [x] Simulated gateway proxy service (happy path: immediate success)
- [x] Payment initiation: create attempt → call gateway → transition state
- [x] `POST /v1/payments/{id}/capture` — capture from authorized
- [x] Auto-capture scheduler (Redis timer or DB sweeper)
- [x] Auth window expiry sweeper (auto-refund uncaptured payments)
- [x] Outbox: write `payment.authorized`, `payment.captured`, `payment.failed`
- [x] Connect to order service: update order status on capture
- [x] Unit tests: state machine (all transitions + invalid transitions)
- [x] Unit tests: auto-capture logic, expiry logic
- [x] Integration test: order → payment → capture full flow

### Checkout (simulated)
- [x] Simple HTML checkout page that submits payment against an order
- [x] Checkout verifies order exists and is not expired
- [x] Callback URL handling: redirect after payment

### Ledger service (Phase 1 — basic)
- [x] Ledger accounts table with seed data
- [x] Ledger entry creation with double-entry validation
- [x] gRPC endpoint: `CreateEntries(transaction_id, entries[])`
- [x] Balance query: sum debits and credits per account
- [x] Unit tests: debit == credit constraint, single-side constraint
- [x] Integration test: capture creates correct ledger entries

### API gateway (basic)
- [x] Request routing to backend services
- [x] API key authentication (delegates to auth package)
- [x] Correlation ID injection (`X-Request-Id`)
- [x] Request/response logging (structured JSON, scrubbed)
- [x] Basic rate limiting (per-merchant, per-endpoint)

### Dashboard (Phase 1)
- [x] Login page (merchant user auth)
- [x] Orders list page with pagination
- [x] Order detail page
- [x] Payment detail page with state history
- [x] API key management page (create, view, revoke)
- [x] Basic layout with navigation

### Phase 1 milestone tests
- [x] Can create a merchant and generate API keys
- [x] Can create an order via API
- [x] Can simulate a payment through checkout
- [x] Payment moves through created → authorized → captured
- [x] Capture creates ledger entries with correct amounts
- [x] Order transitions to `paid` after capture
- [x] Dashboard shows orders and payments

---

## Phase 2 — Reliability and money movement

### Idempotency
- [x] Idempotency middleware (Redis SET NX EX + durable Postgres records for money-changing writes)
- [x] Reject same idempotency key with different request hash
- [x] Handle all three cases: new, completed, in-progress
- [x] `Idempotent-Replayed: true` header on cached responses
- [x] `409 Conflict` with `Retry-After` for in-progress requests
- [x] Apply to all POST endpoints
- [x] Unit tests: all three idempotency cases
- [x] Integration test: duplicate request returns same response

### Outbox relay
- [x] Outbox relay worker (polls outbox table, publishes to Kafka)
- [x] Polling with `FOR UPDATE SKIP LOCKED`
- [x] Mark `published_at` on successful publish
- [x] Retry logic on Kafka publish failure
- [x] Cleanup job (delete published entries > 7 days)
- [x] Health metric: unpublished entry count
- [x] Integration test: event appears in Kafka after outbox insert

### Refund service
- [x] Refund domain model with state machine
- [x] `POST /v1/payments/{id}/refunds` — create refund
- [x] `GET /v1/refunds/{id}` — fetch refund
- [x] `GET /v1/payments/{id}/refunds` — list refunds for payment
- [x] Full and partial refund support
- [x] Refund eligibility validation with row-level locking
- [x] Concurrent refund protection (`SELECT FOR UPDATE`)
- [x] Async refund processing (queue → gateway → status update)
- [x] Reserve refund amount on creation via `amount_refunded_pending`
- [x] Create refund reversal ledger entries only after gateway confirms `processed`
- [x] Release pending refund reservation on `failed`
- [x] Update `payment.amount_refunded` and `payment.refund_status`
- [x] Outbox: `refund.created`, `refund.processed`, `refund.failed`
- [x] Unit tests: eligibility checks, concurrent refund prevention
- [x] Integration test: capture → refund → verify ledger balances

### Webhook service
- [x] Webhook subscription CRUD: create, list, update, delete
- [x] Kafka consumer: subscribe to all `paygate.*` topics
- [x] Event-to-subscription matching (by event type)
- [x] Signature generation: HMAC-SHA256 of raw payload
- [x] HTTP POST delivery with timeout (10s)
- [x] Delivery attempt recording
- [x] Retry engine (exponential backoff, Redis sorted set)
- [x] Retry worker (polls sorted set, re-delivers)
- [x] Dead-letter queue (after 18 attempts)
- [x] Duplicate suppression (Redis fingerprint, 48h TTL)
- [x] `POST /v1/webhooks/events/{event_id}/replay` — manual replay
- [x] Webhook secret rotation endpoint
- [x] Unit tests: signature generation/verification, retry scheduling
- [x] Integration test: capture → webhook delivered to mock endpoint

### Settlement service
- [x] Settlement domain model with state machine
- [x] Nightly batch job: collect eligible payments
- [x] Fee calculation per payment
- [x] Net amount computation (gross - fees - refunds)
- [x] Settlement and settlement_items creation
- [x] Ledger entries for settlement
- [x] Mark payments as settled
- [x] Settlement hold/release mechanism
- [x] Outbox: `settlement.created`, `settlement.processed`
- [x] `GET /v1/settlements` — list settlements for merchant
- [x] `GET /v1/settlements/{id}` — settlement detail with items
- [x] Unit tests: fee calculation, net amount computation
- [x] Integration test: capture multiple payments → run settlement → verify

### Reconciliation worker
- [x] Three-way match: payment ↔ ledger ↔ settlement
- [x] Mismatch detection and classification
- [x] Reconciliation batch recording
- [x] Continuous ledger balance check (every 5 min)
- [x] Hourly payment-to-ledger recon
- [x] Nightly full three-way recon
- [x] Mismatch alerting
- [x] Integration test: inject intentional mismatches → verify detection

### Dashboard (Phase 2)
- [x] Refund console (issue refund, view status)
- [x] Webhook delivery log (per event, per subscription)
- [x] Webhook subscription management
- [x] Settlement reports page
- [x] Reconciliation dashboard (mismatch summary)

### Phase 2 milestone tests
- [x] Idempotent requests work correctly across all POST endpoints
- [x] Outbox relay publishes events within 500ms of state change
- [x] Full and partial refunds work with correct ledger entries
- [x] Concurrent refund requests don't exceed captured amount
- [x] Webhooks delivered within 5s of event creation
- [x] Failed webhooks retry with correct backoff schedule
- [x] Dead-lettered webhooks can be replayed
- [x] Settlement batch correctly groups and calculates
- [x] Reconciliation detects intentionally planted mismatches
- [x] Ledger balance check passes (debits = credits)

---

## Phase 3 — Risk and controls

### Risk engine
- [x] Velocity check: per-merchant transaction rate (configurable threshold)
- [x] Velocity check: per-IP payment attempts
- [x] Velocity check: per-card/token payment attempts
- [x] Amount spike detection (> 3x average transaction)
- [x] Rule-based risk scoring (configurable rules per merchant)
- [x] Risk hold: flag payment for manual review before capture
- [x] Manual review queue: approve or reject flagged payments
- [x] Risk event recording
- [x] Risk alerts

### Access control
- [x] RBAC: admin, developer, readonly, ops roles
- [x] Permission matrix per role per endpoint
- [x] API key scope enforcement (read key can't capture)
- [x] Team invitation flow
- [x] IP allowlisting per API key (optional)
- [x] Session management for dashboard users

### Audit logging
- [x] Audit event on every state mutation
- [x] Audit event on every auth event (login, key creation, key revocation)
- [x] Audit event includes: actor, action, resource, changes, IP, correlation ID
- [x] Audit log query API for ops
- [x] Audit log retention and archival (→ S3 after 90 days)

### Security hardening
- [x] Webhook secret rotation with grace period
- [x] API key rotation flow (create new → migrate → revoke old)
- [x] Request scrubbing: strip card numbers, CVV, secrets from logs
- [x] Rate limit tuning per merchant tier
- [x] Input validation: max payload size, field length limits

### Dashboard (Phase 3)
- [x] Risk events page
- [x] Manual review queue
- [x] Audit log viewer
- [x] Team management (invite, roles)
- [x] IP allowlist configuration

---

## Phase 4 — Enterprise-grade operations

### Dispute management
- [ ] Dispute domain model (chargeback lifecycle)
- [ ] Dispute creation (from simulated bank notification)
- [ ] Dispute states: `open → under_review → won | lost | accepted`
- [ ] Evidence submission mechanism
- [ ] Dispute affects settlement holds

### Advanced settlement
- [ ] Partial settlements
- [ ] Configurable settlement cycles per merchant
- [ ] Settlement holds dashboard
- [ ] Payout workflow (settlement → bank transfer simulation)

### Gateway simulator enhancements
- [ ] Configurable scenarios via API (slow, flaky, timeout, duplicate, late)
- [ ] Per-merchant gateway configuration
- [ ] Payment method simulator (card, UPI, netbanking, wallet)

### Observability
- [ ] Grafana dashboards: payment funnel, webhook delivery, settlement
- [ ] OpenTelemetry: full distributed tracing across all services
- [ ] Prometheus metrics: custom business metrics (capture rate, refund rate, etc.)
- [ ] Alerting rules for all P1/P2 conditions
- [ ] Correlation ID search across services

### Chaos testing
- [ ] Toxiproxy setup for inter-service fault injection
- [ ] Chaos test: DB failure during capture
- [ ] Chaos test: Kafka broker failure
- [ ] Chaos test: Redis failure (DB-backed idempotency for money writes, fail-open only for low-risk cache paths)
- [ ] Chaos test: webhook endpoint slow/down
- [ ] Chaos test: outbox relay crash and recovery
- [ ] Document results in runbook

### Load testing
- [ ] k6 scripts for all critical endpoints
- [ ] Baseline performance: 1000 orders/sec
- [ ] Spike test: 5x normal load for 5 minutes
- [ ] Soak test: sustained load for 1 hour
- [ ] Performance regression check in CI (smoke load test)

### Dashboard (Phase 4)
- [ ] Dispute management console
- [ ] Settlement holds/release UI
- [ ] Observability dashboards (embedded Grafana or custom)
- [ ] Gateway simulator control panel
- [ ] Reconciliation drill-down with mismatch details

---

## Phase 5 — Advanced distributed systems track

### Saga orchestration and extraction
- [ ] Add `saga_instances`, `saga_steps`, and `processed_commands` tables
- [ ] Build saga orchestrator service with replay endpoint
- [ ] Add idempotent command handlers in extracted services
- [ ] Implement compensation flows for failed saga branches
- [ ] Integration test: crash/restart in middle of saga without double-posting

### Event schema governance
- [ ] Add schema registry APIs and persistence (`event_schemas`)
- [ ] Add CI compatibility check for producer schema changes
- [ ] Add consumer contract test gate before schema activation
- [ ] Support dual-publish rollout and cutover tracking

### Ledger holds and payout rail simulation
- [ ] Add `ledger_holds` table and hold APIs (create/release/commit)
- [ ] Enforce payout eligibility checks against active holds
- [ ] Build payout rail simulator with async callbacks and returns
- [ ] Integration test: hold commit produces exactly one final posting

### Disaster recovery maturity
- [ ] Run quarterly DR drill in staging
- [ ] Measure and record RTO/RPO and replay duration
- [ ] Verify post-restore reconciliation before reopening settlements
- [ ] Add DR drill artifact checklist to runbook

---

## Documentation deliverables

- [ ] API reference (OpenAPI 3.0 spec)
- [ ] Webhook event catalog with JSON schemas
- [ ] Integration guide (how a merchant integrates)
- [ ] Deployment guide (Docker Compose and K8s)
- [ ] Runbook: common operational procedures
- [ ] Architecture decision records (ADRs) for key decisions
- [ ] README.md with quick start, screenshots, and demo instructions

---

## Definition of "done" for each phase

**Phase 1 is done when**: you can create an order via API, pay through checkout, capture the payment, see ledger entries, and view it all on the dashboard.

**Phase 2 is done when**: refunds work, webhooks deliver reliably with retries, settlements group and calculate correctly, reconciliation passes with zero mismatches on the happy path, and idempotency prevents all duplicate operations.

**Phase 3 is done when**: RBAC restricts access correctly, audit logs capture every mutation, risk rules can flag and hold suspicious payments, and API keys can be rotated without downtime.

**Phase 4 is done when**: you can demo the full lifecycle (order → payment → refund → settlement → dispute), show resilience under chaos testing, present load test results, and walk someone through a reconciliation mismatch investigation using the dashboard.
