# PayGate — Implementation Checklist

> Track your progress phase by phase. Every checkbox maps to a concrete deliverable. Don't skip ahead.

---

## Phase 0 — Project setup

### Repository and tooling
- [ ] Initialize Go module: `go mod init github.com/{you}/paygate`
- [ ] Set up monorepo structure (see directory layout below)
- [ ] Configure `golangci-lint` with strict config
- [ ] Set up `Makefile` with targets: `build`, `test`, `lint`, `migrate`, `dev`, `docker`
- [ ] Create `docker-compose.yml` with Postgres, Kafka (KRaft), Redis, MinIO
- [ ] Create `docker-compose.test.yml` for integration tests
- [ ] Set up database migration tool (`golang-migrate`)
- [ ] Create initial migration: merchant and API key tables
- [ ] Write `KSUID` ID generator with prefix support
- [ ] Set up structured JSON logger (zerolog or slog)
- [ ] Set up OpenTelemetry tracing (basic, wire through HTTP/gRPC)
- [ ] Create health check endpoints (`/healthz`, `/readyz`) as shared middleware
- [ ] Initialize Next.js frontend in `dashboard/` directory
- [ ] Set up CI pipeline (lint → unit test → build → integration test)

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
- [ ] Merchant registration endpoint: `POST /v1/merchants`
- [ ] Merchant model with settings (auto_capture, fee_rate, etc.)
- [ ] API key generation: `POST /v1/merchants/{id}/keys`
- [ ] API key authentication middleware (Basic auth, bcrypt verification)
- [ ] API key scoping (read, write, admin)
- [ ] API key revocation endpoint
- [ ] Unit tests: auth middleware, key validation, scope checking

### Order service
- [ ] Order domain model with state machine
- [ ] `POST /v1/orders` — create order
- [ ] `GET /v1/orders/{id}` — fetch order
- [ ] `GET /v1/orders` — list orders (cursor pagination)
- [ ] Order expiry: set `expires_at` on creation (default 30 min)
- [ ] Order expiry sweeper (CronJob or ticker goroutine)
- [ ] Outbox: write `order.created` event in same transaction
- [ ] Unit tests: state machine transitions, validation, pagination
- [ ] Integration test: create order → verify in DB

### Payment service
- [ ] Payment attempt model
- [ ] Payment domain model with state machine
- [ ] Simulated gateway proxy service (happy path: immediate success)
- [ ] Payment initiation: create attempt → call gateway → transition state
- [ ] `POST /v1/payments/{id}/capture` — capture from authorized
- [ ] Auto-capture scheduler (Redis timer or DB sweeper)
- [ ] Auth window expiry sweeper (auto-refund uncaptured payments)
- [ ] Outbox: write `payment.authorized`, `payment.captured`, `payment.failed`
- [ ] Connect to order service: update order status on capture
- [ ] Unit tests: state machine (all transitions + invalid transitions)
- [ ] Unit tests: auto-capture logic, expiry logic
- [ ] Integration test: order → payment → capture full flow

### Checkout (simulated)
- [ ] Simple HTML checkout page that submits payment against an order
- [ ] Checkout verifies order exists and is not expired
- [ ] Callback URL handling: redirect after payment

### Ledger service (Phase 1 — basic)
- [ ] Ledger accounts table with seed data
- [ ] Ledger entry creation with double-entry validation
- [ ] gRPC endpoint: `CreateEntries(transaction_id, entries[])`
- [ ] Balance query: sum debits and credits per account
- [ ] Unit tests: debit == credit constraint, single-side constraint
- [ ] Integration test: capture creates correct ledger entries

### API gateway (basic)
- [ ] Request routing to backend services
- [ ] API key authentication (delegates to auth package)
- [ ] Correlation ID injection (`X-Request-Id`)
- [ ] Request/response logging (structured JSON, scrubbed)
- [ ] Basic rate limiting (per-merchant, per-endpoint)

### Dashboard (Phase 1)
- [ ] Login page (merchant user auth)
- [ ] Orders list page with pagination
- [ ] Order detail page
- [ ] Payment detail page with state history
- [ ] API key management page (create, view, revoke)
- [ ] Basic layout with navigation

### Phase 1 milestone tests
- [ ] Can create a merchant and generate API keys
- [ ] Can create an order via API
- [ ] Can simulate a payment through checkout
- [ ] Payment moves through created → authorized → captured
- [ ] Capture creates ledger entries with correct amounts
- [ ] Order transitions to `paid` after capture
- [ ] Dashboard shows orders and payments

---

## Phase 2 — Reliability and money movement

### Idempotency
- [ ] Idempotency middleware (Redis SET NX EX)
- [ ] Handle all three cases: new, completed, in-progress
- [ ] `Idempotent-Replayed: true` header on cached responses
- [ ] `409 Conflict` with `Retry-After` for in-progress requests
- [ ] Apply to all POST endpoints
- [ ] Unit tests: all three idempotency cases
- [ ] Integration test: duplicate request returns same response

### Outbox relay
- [ ] Outbox relay worker (polls outbox table, publishes to Kafka)
- [ ] Polling with `FOR UPDATE SKIP LOCKED`
- [ ] Mark `published_at` on successful publish
- [ ] Retry logic on Kafka publish failure
- [ ] Cleanup job (delete published entries > 7 days)
- [ ] Health metric: unpublished entry count
- [ ] Integration test: event appears in Kafka after outbox insert

### Refund service
- [ ] Refund domain model with state machine
- [ ] `POST /v1/payments/{id}/refunds` — create refund
- [ ] `GET /v1/refunds/{id}` — fetch refund
- [ ] `GET /v1/payments/{id}/refunds` — list refunds for payment
- [ ] Full and partial refund support
- [ ] Refund eligibility validation with row-level locking
- [ ] Concurrent refund protection (`SELECT FOR UPDATE`)
- [ ] Async refund processing (queue → gateway → status update)
- [ ] Compensating ledger entries on refund processed
- [ ] Update `payment.amount_refunded` and `payment.refund_status`
- [ ] Outbox: `refund.created`, `refund.processed`, `refund.failed`
- [ ] Unit tests: eligibility checks, concurrent refund prevention
- [ ] Integration test: capture → refund → verify ledger balances

### Webhook service
- [ ] Webhook subscription CRUD: create, list, update, delete
- [ ] Kafka consumer: subscribe to all `paygate.*` topics
- [ ] Event-to-subscription matching (by event type)
- [ ] Signature generation: HMAC-SHA256 of raw payload
- [ ] HTTP POST delivery with timeout (10s)
- [ ] Delivery attempt recording
- [ ] Retry engine (exponential backoff, Redis sorted set)
- [ ] Retry worker (polls sorted set, re-delivers)
- [ ] Dead-letter queue (after 18 attempts)
- [ ] Duplicate suppression (Redis fingerprint, 48h TTL)
- [ ] `POST /v1/webhooks/{event_id}/replay` — manual replay
- [ ] Webhook secret rotation endpoint
- [ ] Unit tests: signature generation/verification, retry scheduling
- [ ] Integration test: capture → webhook delivered to mock endpoint

### Settlement service
- [ ] Settlement domain model with state machine
- [ ] Nightly batch job: collect eligible payments
- [ ] Fee calculation per payment
- [ ] Net amount computation (gross - fees - refunds)
- [ ] Settlement and settlement_items creation
- [ ] Ledger entries for settlement
- [ ] Mark payments as settled
- [ ] Settlement hold/release mechanism
- [ ] Outbox: `settlement.created`, `settlement.processed`
- [ ] `GET /v1/settlements` — list settlements for merchant
- [ ] `GET /v1/settlements/{id}` — settlement detail with items
- [ ] Unit tests: fee calculation, net amount computation
- [ ] Integration test: capture multiple payments → run settlement → verify

### Reconciliation worker
- [ ] Three-way match: payment ↔ ledger ↔ settlement
- [ ] Mismatch detection and classification
- [ ] Reconciliation batch recording
- [ ] Continuous ledger balance check (every 5 min)
- [ ] Hourly payment-to-ledger recon
- [ ] Nightly full three-way recon
- [ ] Mismatch alerting
- [ ] Integration test: inject intentional mismatches → verify detection

### Dashboard (Phase 2)
- [ ] Refund console (issue refund, view status)
- [ ] Webhook delivery log (per event, per subscription)
- [ ] Webhook subscription management
- [ ] Settlement reports page
- [ ] Reconciliation dashboard (mismatch summary)

### Phase 2 milestone tests
- [ ] Idempotent requests work correctly across all POST endpoints
- [ ] Outbox relay publishes events within 500ms of state change
- [ ] Full and partial refunds work with correct ledger entries
- [ ] Concurrent refund requests don't exceed captured amount
- [ ] Webhooks delivered within 5s of event creation
- [ ] Failed webhooks retry with correct backoff schedule
- [ ] Dead-lettered webhooks can be replayed
- [ ] Settlement batch correctly groups and calculates
- [ ] Reconciliation detects intentionally planted mismatches
- [ ] Ledger balance check passes (debits = credits)

---

## Phase 3 — Risk and controls

### Risk engine
- [ ] Velocity check: per-merchant transaction rate (configurable threshold)
- [ ] Velocity check: per-IP payment attempts
- [ ] Velocity check: per-card/token payment attempts
- [ ] Amount spike detection (> 3x average transaction)
- [ ] Rule-based risk scoring (configurable rules per merchant)
- [ ] Risk hold: flag payment for manual review before capture
- [ ] Manual review queue: approve or reject flagged payments
- [ ] Risk event recording
- [ ] Risk alerts

### Access control
- [ ] RBAC: admin, developer, readonly, ops roles
- [ ] Permission matrix per role per endpoint
- [ ] API key scope enforcement (read key can't capture)
- [ ] Team invitation flow
- [ ] IP allowlisting per API key (optional)
- [ ] Session management for dashboard users

### Audit logging
- [ ] Audit event on every state mutation
- [ ] Audit event on every auth event (login, key creation, key revocation)
- [ ] Audit event includes: actor, action, resource, changes, IP, correlation ID
- [ ] Audit log query API for ops
- [ ] Audit log retention and archival (→ S3 after 90 days)

### Security hardening
- [ ] Webhook secret rotation with grace period
- [ ] API key rotation flow (create new → migrate → revoke old)
- [ ] Request scrubbing: strip card numbers, CVV, secrets from logs
- [ ] Rate limit tuning per merchant tier
- [ ] Input validation: max payload size, field length limits

### Dashboard (Phase 3)
- [ ] Risk events page
- [ ] Manual review queue
- [ ] Audit log viewer
- [ ] Team management (invite, roles)
- [ ] IP allowlist configuration

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
- [ ] Chaos test: Redis failure (fail-open behavior)
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
