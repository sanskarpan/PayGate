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
- [x] Dispute domain model (chargeback lifecycle)
- [x] Dispute creation (from simulated bank notification)
- [x] Dispute states: `open → under_review → won | lost | accepted`
- [x] Evidence submission mechanism
- [x] Dispute affects settlement holds

### Advanced settlement
- [x] Partial settlements
- [x] Configurable settlement cycles per merchant
- [x] Settlement holds dashboard
- [x] Payout workflow (settlement → bank transfer simulation)

### Gateway simulator enhancements
- [x] Configurable scenarios via API (slow, flaky, timeout, duplicate, late)
- [x] Per-merchant gateway configuration
- [x] Payment method simulator (card, UPI, netbanking, wallet)

### Observability
- [x] Grafana dashboards: payment funnel, webhook delivery, settlement
- [x] OpenTelemetry: full distributed tracing across all services
- [x] Prometheus metrics: custom business metrics (capture rate, refund rate, etc.)
- [x] Alerting rules for all P1/P2 conditions
- [x] Correlation ID search across services

### Chaos testing
- [x] Toxiproxy setup for inter-service fault injection
- [x] Chaos test: DB failure during capture
- [x] Chaos test: Kafka broker failure
- [x] Chaos test: Redis failure (DB-backed idempotency for money writes, fail-open only for low-risk cache paths)
- [x] Chaos test: webhook endpoint slow/down
- [x] Chaos test: outbox relay crash and recovery
- [x] Document results in runbook

### Load testing
- [x] k6 scripts for all critical endpoints
- [x] Baseline performance: 1000 orders/sec
- [x] Spike test: 5x normal load for 5 minutes
- [x] Soak test: sustained load for 1 hour
- [x] Performance regression check in CI (smoke load test)

### Dashboard (Phase 4)
- [x] Dispute management console
- [x] Settlement holds/release UI
- [x] Observability dashboards (embedded Grafana or custom)
- [x] Gateway simulator control panel
- [x] Reconciliation drill-down with mismatch details

---

## Phase 5 — Advanced distributed systems track

### Saga orchestration and extraction
- [x] Define extraction boundaries and target service ownership before moving code out of the modular monolith
- [x] Write ADRs for the first extracted flows: order orchestration, payment orchestration, refund orchestration, settlement finalization
- [x] Document command/event ownership for each extracted service: who emits, who consumes, who is source of truth
- [x] Define per-service data ownership so no table ends up with ambiguous write authority
- [x] Add `saga_instances` table with status, correlation IDs, causation IDs, timeout fields, replay metadata, and failure reason fields
- [x] Add `saga_steps` table with deterministic step ordering, step type (`command`, `wait`, `compensation`), retry counters, leased worker identity, and terminal status
- [x] Add `processed_commands` table keyed by consumer + command ID to guarantee command-side idempotency across retries and restarts
- [x] Add `saga_timeouts` or equivalent scheduling mechanism for timeout-driven compensations and stalled-flow recovery
- [x] Define a single canonical saga state machine: `pending -> running -> waiting -> compensating -> completed | failed | aborted`
- [x] Build a dedicated saga orchestrator service/process instead of burying orchestration in request handlers
- [x] Add command dispatch abstraction with persistent command envelopes and explicit ack/nack semantics
- [x] Add per-step retry policies with max attempts, backoff strategy, poison-message handling, and operator-visible error classification
- [x] Persist full step input/output snapshots or references so replay/debugging does not depend on ephemeral logs alone
- [x] Add saga replay endpoint for operators with dry-run mode, force mode, and audit logging
- [x] Add saga inspection endpoint showing current state, completed steps, blocked step, retry history, and compensation state
- [x] Add lease-based worker coordination so the same saga step is not executed concurrently by multiple orchestrator instances
- [x] Add stale-lease recovery for orchestrator crashes and network partitions
- [x] Add idempotent command handlers in each extracted service with explicit duplicate detection and safe re-entry behavior
- [x] Ensure every side-effecting command writes durable state before acknowledging completion to the orchestrator
- [x] Implement compensation flows for every non-atomic business branch: auth reversal, refund reversal, settlement rollback marker, payout cancellation, webhook replay cancellation where applicable
- [x] Define compensation semantics explicitly for steps that are not truly reversible and require forward-only remediation instead
- [x] Add operator override flow for sagas that cannot auto-compensate cleanly
- [x] Add dead-letter handling for permanently failed saga commands/events with dashboard visibility and replay tooling
- [x] Add integration tests for crash/restart in the middle of a saga without duplicate postings or duplicate external side effects
- [x] Add integration tests for at-least-once delivery, out-of-order delivery, duplicate commands, and delayed compensation messages
- [x] Add migration/rollout plan for extracting the first service without breaking existing monolith endpoints during transition

### Event schema governance
- [x] Define event versioning policy: additive-only rules, breaking-change review rules, deprecation windows, and ownership approval flow
- [x] Add schema registry APIs and persistence (`event_schemas`, `schema_versions`, `schema_compatibility_checks`, `schema_rollouts`)
- [x] Store canonical schema metadata: subject, version, status (`draft`, `active`, `deprecated`, `retired`), owner, review link, and activation timestamp
- [x] Add schema linting in CI for required envelope fields: event ID, event type, occurred-at, correlation ID, causation ID, schema version
- [x] Add backward-compatibility checks for producer schema changes
- [x] Add forward-compatibility checks where consumers are expected to tolerate future optional fields
- [x] Add CI gate that blocks merging incompatible producer schema changes without explicit override and approval
- [x] Add consumer contract test suite that validates real consumer deserialization/validation against candidate schemas before activation
- [x] Add sample payload fixtures per event type and require updates when schemas evolve
- [x] Add dual-publish rollout support so producers can emit old and new schema versions during migration windows
- [x] Add cutover tracking showing when every consumer has acknowledged support for the new schema
- [x] Add registry APIs/UI to compare versions and explain incompatibilities in human-readable form
- [x] Add runtime metric for schema version distribution per topic and per consumer group
- [x] Add alerting when deprecated schema versions remain in heavy use past cutover deadline
- [x] Add operator runbook for emergency schema rollback and consumer freeze procedures
- [x] Add replay safety rules so historical events are interpreted against the correct schema version instead of current defaults

### Ledger holds and payout rail simulation
- [x] Define hold lifecycle semantics separately from settlement holds: authorization hold, reserve hold, compliance hold, payout hold, dispute hold
- [x] Add `ledger_holds` table with owner type, owner ID, reason, amount, currency, expiry, status, and idempotency fields
- [x] Add hold ledger references so operators can trace hold creation/release/commit back to business objects and audit records
- [x] Build hold APIs: create, extend, release, commit, expire, inspect
- [x] Enforce single-writer rules and row-level locking around hold mutations
- [x] Ensure hold commit is atomic with final ledger posting so funds cannot be released and posted twice
- [x] Ensure hold release is idempotent and safe under retries or callback duplication
- [x] Add sweeper for expired holds with policy-driven behavior: auto-release, escalate, or mark for manual review
- [x] Enforce payout eligibility checks against both settlement-level holds and active ledger holds
- [x] Prevent payout initiation when aggregate committed/held amounts exceed available merchant balance
- [x] Add balance projection logic that distinguishes posted balance, reserved balance, releasable balance, and payoutable balance
- [x] Build payout rail simulator with asynchronous callbacks for `processing`, `completed`, `failed`, `returned`, and `reversed`
- [x] Simulate rail-specific delays, duplicate callbacks, out-of-order callbacks, partial failures, and bank return reasons
- [x] Add callback authenticity checks and replay protection for simulated payout rail notifications
- [x] Model payout return workflows that create corrective ledger entries without corrupting prior settlement history
- [x] Add operator tooling to view payout timeline: initiated, sent, callback received, completed/returned, corrective action taken
- [x] Add integration tests proving hold commit produces exactly one final posting even under retries, restarts, and duplicate callbacks
- [x] Add integration tests proving payout returns and reversals preserve ledger invariants and merchant balance correctness

### Disaster recovery maturity
- [x] Define formal DR scope: Postgres, Kafka, Redis, MinIO, schema registry, dashboard session store, secrets/config, and deployment manifests
- [x] Document backup cadence, retention, encryption, integrity verification, and restore ownership for every stateful dependency
- [x] Define target RTO/RPO per subsystem instead of one generic platform-wide value
- [x] Automate backup verification so snapshots are not assumed valid without restore testing
- [x] Build restore playbooks for single-service restore, full-region restore, and partial data corruption scenarios
- [x] Add infrastructure-as-code procedure for recreating the full environment from clean state
- [x] Run quarterly DR drill in staging with production-like data volume and async backlog volume
- [x] Run at least one game-day scenario for each class of failure: database loss, Kafka topic loss, Redis loss, object-store loss, orchestrator crash-loop
- [x] Measure and record RTO, RPO, backlog replay duration, and manual intervention count for every drill
- [x] Verify post-restore reconciliation before reopening settlements, payouts, or dispute processing
- [x] Verify idempotency store correctness after restore so replayed writes do not double-post money movements
- [x] Verify outbox replay and webhook replay behavior after restore, including duplicate suppression behavior
- [x] Add explicit signoff checklist before reopening merchant-facing traffic after DR
- [x] Add DR drill artifact checklist to runbook: timeline, screenshots, metrics, blocking issues, remediation owner, next scheduled drill
- [x] Create follow-up issue generation process so every drill produces tracked remediation work instead of tribal knowledge only

### Operability, rollout safety, and production controls
- [x] Add cross-service correlation strategy so every command, event, saga, webhook, payout, and ledger mutation can be traced end to end
- [x] Add per-service SLOs and error budgets for command latency, event publish latency, orchestration lag, payout callback latency, and replay completion time
- [x] Add dashboards for saga backlog, stuck compensations, duplicate command rate, schema version spread, hold aging, and replay queue depth
- [x] Add alerts for stale saga leases, poison commands, schema activation failures, hold expiry backlog, and replay lag beyond threshold
- [x] Add feature flags for extracted-service cutovers, dual-write/dual-read windows, and emergency fallback to monolith-owned flow
- [x] Add shadow-mode execution path for first service extraction so outputs can be compared before taking ownership live
- [x] Add compare-and-diff tooling for shadow-mode runs to detect behavior drift between monolith and extracted service
- [x] Add rollout checklist for first extraction: dark launch, shadow traffic, partial traffic, full cutover, rollback conditions
- [x] Add explicit kill-switches for saga dispatch, payout callbacks, webhook replay, and schema activation
- [x] Add immutable audit logging for operator-triggered replay, compensation, override, and force-complete actions

### Phase 5 milestone tests
- [x] Can extract one business flow behind a stable contract without changing merchant-facing API behavior
- [x] Can restart the orchestrator or a participant service mid-saga without duplicate commands, duplicate postings, or lost compensations
- [x] Can replay a failed saga from persisted state and reach the same terminal outcome deterministically
- [x] Schema registry rejects incompatible producer changes in CI before they ship
- [x] Consumers can prove readiness for a new schema version before producer cutover
- [x] Dual-publish rollout can be observed end to end and retired cleanly after cutover
- [x] Ledger holds prevent funds release/payout until explicitly committed or released
- [x] Hold commit/release remains exactly-once under duplicate requests and duplicate callbacks
- [x] Payout rail simulator can drive success, delayed success, return, reverse, and duplicate callback scenarios without breaking balances
- [x] Full restore drill can recover the platform within target RTO/RPO and pass reconciliation before traffic reopening
- [x] Operators can inspect, replay, compensate, and override failed distributed flows from tooling rather than direct database edits

---

## Documentation deliverables

- [x] API reference (OpenAPI 3.0 spec)
- [x] Webhook event catalog with JSON schemas
- [x] Integration guide (how a merchant integrates)
- [x] Deployment guide (Docker Compose and K8s)
- [x] Runbook: common operational procedures
- [x] Architecture decision records (ADRs) for key decisions
- [x] README.md with quick start, screenshots, and demo instructions

---

## Definition of "done" for each phase

**Phase 1 is done when**: you can create an order via API, pay through checkout, capture the payment, see ledger entries, and view it all on the dashboard.

**Phase 2 is done when**: refunds work, webhooks deliver reliably with retries, settlements group and calculate correctly, reconciliation passes with zero mismatches on the happy path, and idempotency prevents all duplicate operations.

**Phase 3 is done when**: RBAC restricts access correctly, audit logs capture every mutation, risk rules can flag and hold suspicious payments, and API keys can be rotated without downtime.

**Phase 4 is done when**: you can demo the full lifecycle (order → payment → refund → settlement → dispute), show resilience under chaos testing, present load test results, and walk someone through a reconciliation mismatch investigation using the dashboard.

**Phase 5 is done when**: at least one core flow has been safely extracted behind durable command/event contracts, orchestration and compensation are deterministic under retries and restarts, schema evolution is governed by enforced compatibility gates, ledger holds and payout rail callbacks preserve exactly-once money movement semantics, and a full DR drill can restore the platform and pass reconciliation before reopening traffic.
