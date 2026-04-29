# PayGate — Architecture

> System architecture, service boundaries, infrastructure, and deployment topology.

---

## 1. Architecture style

**Service-oriented modular monolith first, extractable microservices later.** The documentation describes service boundaries because they are the right domain seams, but the first production-quality implementation should run money-critical paths inside one Go backend process and one PostgreSQL transaction boundary. Kafka remains the event backbone for asynchronous consumers. The frontend is a separate Next.js application that talks to an API gateway.

This is not a microservices-for-the-sake-of-microservices design. Each boundary aligns with a distinct operational concern, but boundaries are extracted only when the consistency model is explicit and tested. For a portfolio project, correctness beats distributed complexity.

### 1.1 Implementation posture

| Phase | Runtime shape | Why |
|-------|---------------|-----|
| Phase 1 | Modular monolith: API, order, payment, ledger, outbox, and auth packages in one deployable | Gives one DB transaction for capture + ledger + outbox, eliminating dual-write risk |
| Phase 2 | Split workers: outbox relay, webhook worker, settlement worker, recon worker | Workers are async and safe to isolate because Postgres/Kafka provide replay |
| Phase 3+ | Optional service extraction behind gRPC | Extract only after idempotent command handling, retries, and reconciliation are proven |
| Phase 5 (advanced) | Ledger extraction + saga orchestrator + schema registry | Enables distributed ownership while preserving money invariants |

The directory can still be service-shaped. The important rule is that the synchronous money path must not depend on best-effort cross-service writes.

---

## 2. Logical service map

This map shows domain boundaries and future extraction seams. In Phase 1 these run mostly inside `cmd/api` plus separate async workers.

```
┌─────────────────────────────────────────────────────────────────────┐
│                          API Gateway                                │
│  (rate limiting, auth, routing, request logging, correlation ID)    │
└──────────┬─────────────┬──────────────┬──────────────┬──────────────┘
           │             │              │              │
     ┌─────▼─────┐ ┌────▼─────┐ ┌─────▼──────┐ ┌────▼──────┐
     │  Order    │ │ Payment  │ │  Refund    │ │ Settlement│
     │  Service  │ │ Service  │ │  Service   │ │ Service   │
     └─────┬─────┘ └────┬─────┘ └─────┬──────┘ └────┬──────┘
           │             │              │              │
           └──────┬──────┴──────┬───────┴──────┬───────┘
                  │             │              │
           ┌──────▼──────┐ ┌───▼───────┐ ┌────▼────────┐
           │  Ledger     │ │  Webhook  │ │   Recon     │
           │  Service    │ │  Service  │ │   Worker    │
           └─────────────┘ └───────────┘ └─────────────┘

  Shared infrastructure:
  ┌──────────┐ ┌───────────┐ ┌────────────┐ ┌─────────┐
  │PostgreSQL│ │   Kafka   │ │   Redis    │ │   S3    │
  └──────────┘ └───────────┘ └────────────┘ └─────────┘
```

---

## 3. Service responsibilities

### 3.1 API Gateway
- **Technology**: Go (custom) or Kong/Envoy
- **Responsibilities**: TLS termination, rate limiting (token bucket per merchant), API key authentication, request/response logging, correlation ID injection (`X-Request-Id`), request scrubbing, routing to backend services
- **Does NOT**: contain business logic, hold state, access the database directly

### 3.2 Order Service
- **Technology**: Go
- **Database**: `paygate_orders` schema in PostgreSQL
- **Responsibilities**: create orders, validate amounts, track order state, expose order queries
- **Publishes**: `order.created`, `order.paid`, `order.expired`
- **Consumes**: `payment.captured` (to transition order to `paid`)

### 3.3 Payment Service
- **Technology**: Go
- **Database**: `paygate_payments` schema in PostgreSQL
- **Responsibilities**: create payment attempts, call gateway for authorization, handle capture, manage auto-capture scheduler, handle gateway callbacks
- **Publishes**: `payment.authorized`, `payment.captured`, `payment.failed`
- **Calls**: Gateway Proxy (simulated). Uses the ledger module in the same DB transaction on capture.
- **Critical path**: this is the most latency-sensitive service. Optimize for minimal database round-trips during authorization.

### 3.4 Refund Service
- **Technology**: Go
- **Database**: `paygate_refunds` schema in PostgreSQL
- **Responsibilities**: validate refund eligibility (payment must be captured, amount within remaining), create refund records, process refunds via gateway, track refund state
- **Publishes**: `refund.created`, `refund.processed`, `refund.failed`
- **Calls**: Ledger module when a refund is confirmed as processed, in the same DB transaction as the refund status update.

### 3.5 Settlement Service
- **Technology**: Go
- **Database**: `paygate_settlements` schema in PostgreSQL
- **Responsibilities**: nightly batch collection of eligible payments, fee calculation, settlement creation, settlement holds management, payout tracking
- **Publishes**: `settlement.created`, `settlement.processed`
- **Calls**: Ledger module in the same DB transaction as settlement item creation.

### 3.6 Ledger Service
- **Technology**: Go
- **Database**: `paygate_ledger` schema in PostgreSQL (dedicated, no sharing)
- **Responsibilities**: accept journal entry requests, enforce double-entry invariant (debit = credit), provide balance queries, run periodic balance checks
- **Important**: this module/service is **append-only** for writes. No UPDATE or DELETE on ledger entries, ever.
- **Money-path rule**: in Phase 1, ledger entries are written in the same PostgreSQL transaction as the payment/refund/settlement state change and outbox event. If Ledger is later extracted as a network service, it must expose idempotent commands and the calling flow must become an explicit saga. Do not combine synchronous network ledger calls with independent payment commits without a recovery protocol.

### 3.7 Webhook Service
- **Technology**: Go
- **Database**: `paygate_webhooks` schema in PostgreSQL
- **Responsibilities**: manage webhook subscriptions, consume events from Kafka, match events to subscriptions, deliver HTTP POSTs, manage retry queue, record delivery attempts, support replay
- **Consumes**: all `paygate.*` Kafka topics
- **Publishes**: `webhook.delivery.exhausted` (internal alert)

### 3.8 Reconciliation Worker
- **Technology**: Go
- **Database**: reads from `paygate_payments`, `paygate_ledger`, `paygate_settlements`
- **Responsibilities**: run scheduled reconciliation jobs, compare payment/ledger/settlement records, detect and classify mismatches, store reconciliation batch results, fire alerts on critical mismatches
- **Publishes**: `recon.mismatch.detected`

### 3.9 Gateway Proxy (Simulated)
- **Technology**: Go
- **Responsibilities**: simulate bank/PSP responses with configurable behavior (success, delay, failure, late callback, duplicates). Accepts authorization and refund requests. Returns simulated responses.
- **Standalone**: no database, no events. Purely a test double with an HTTP API for configuration.

### 3.10 Merchant Dashboard (Frontend)
- **Technology**: Next.js 14+ with App Router, TypeScript, Tailwind CSS, shadcn/ui
- **Pages**: login, orders list, payment details, refund console, webhook delivery log, settlement reports, reconciliation dashboard, API key management, team settings, risk events
- **Auth**: session-based (NextAuth/custom), backed by merchant user table
- **API calls**: through API Gateway only

---

## 4. Data architecture

### 4.1 Database strategy

One PostgreSQL cluster, logically separated schemas per domain. Runtime packages own their schemas. In Phase 1, a single backend role may write multiple schemas only inside money-critical transactions. Outside those explicitly documented transaction boundaries, cross-domain reads should go through query APIs, read models, or reconciliation jobs.

```
PostgreSQL cluster
├── paygate_orders       (Order Service)
├── paygate_payments     (Payment Service)
├── paygate_refunds      (Refund Service)
├── paygate_settlements  (Settlement Service)
├── paygate_ledger       (Ledger Service — most critical)
├── paygate_webhooks     (Webhook Service)
├── paygate_merchants    (shared merchant/auth data)
├── paygate_audit        (Audit events — append-only)
└── paygate_idempotency  (Durable idempotency records)
```

### 4.2 Why not separate databases?

For a portfolio project, a single Postgres cluster with schema isolation gives conceptual separation without the operational overhead of multiple database clusters. In production, split async workers first. Split the ledger last unless you also introduce idempotent command processing, a durable command log, and reconciliation for in-flight ledger commands.

### 4.3 Kafka topics

| Topic | Partitions | Key | Retention |
|-------|-----------|-----|-----------|
| `paygate.orders` | 6 | `merchant_id` | 7 days |
| `paygate.payments` | 12 | `merchant_id` | 7 days |
| `paygate.refunds` | 6 | `merchant_id` | 7 days |
| `paygate.settlements` | 6 | `merchant_id` | 7 days |
| `paygate.internal` | 3 | `event_type` | 3 days |
| `paygate.webhook.retry` | 6 | `subscription_id` | 3 days |
| `paygate.deadletter` | 3 | `event_id` | 30 days |

Partitioned by `merchant_id` to guarantee per-merchant ordering.

### 4.4 Redis usage

| Use case | Key pattern | TTL |
|----------|-------------|-----|
| Idempotency keys | `idempotency:{merchant}:{endpoint}:{key}` | 24h |
| Rate limiting | `ratelimit:{merchant}:{endpoint}:{window}` | 1-60s |
| Webhook dedup | `whdedup:{event_id}:{sub_id}` | 48h |
| Auto-capture timer | `autocapture:{payment_id}` | configurable |
| Session cache | `session:{token}` | 1h |

### 4.5 Object storage (S3 / MinIO)

- Settlement reports (CSV/PDF)
- Reconciliation batch exports
- Audit log archives (after retention rotation)
- API request/response logs (long-term)
- Event schema snapshots and compatibility reports

### 4.6 Advanced distributed components

| Component | Responsibility |
|----------|----------------|
| `saga-orchestrator` | Coordinates long-running payment/refund/settlement workflows across extracted services |
| `schema-registry` | Stores event schemas, version metadata, and compatibility policy |
| `payout-rail-sim` | Simulates asynchronous payout rails with return/failure scenarios |
| `risk-scorer` | Combines rules and model score, emits explainable risk decisions |
| `dr-coordinator` | Runs disaster-recovery drills and verifies catch-up checkpoints |

---

## 5. Infrastructure topology

### 5.1 Kubernetes deployment

```
Namespace: paygate
├── Deployments
│   ├── api-gateway          (2 replicas, HPA: 2-8)
│   ├── order-service        (2 replicas, HPA: 2-6)
│   ├── payment-service      (3 replicas, HPA: 3-10)
│   ├── refund-service       (2 replicas, HPA: 2-4)
│   ├── settlement-service   (1 replica — batch worker)
│   ├── ledger-service       (2 replicas, HPA: 2-4)
│   ├── webhook-service      (3 replicas, HPA: 3-8)
│   ├── recon-worker         (1 replica — scheduled)
│   ├── outbox-relay         (2 replicas — active-passive)
│   ├── gateway-proxy        (1 replica — simulator)
│   ├── saga-orchestrator    (2 replicas — leader election)
│   ├── schema-registry      (2 replicas)
│   ├── risk-scorer          (2 replicas)
│   ├── payout-rail-sim      (1 replica)
│   └── dashboard            (2 replicas)
├── StatefulSets
│   ├── postgresql           (1 primary + 1 replica)
│   ├── kafka                (3 brokers)
│   └── redis                (1 primary + 1 replica)
├── CronJobs
│   ├── settlement-batch     (daily 02:00 UTC)
│   ├── recon-nightly        (daily 04:00 UTC)
│   ├── autocapture-sweeper  (every 5 minutes)
│   └── outbox-cleanup       (daily 05:00 UTC)
└── ConfigMaps / Secrets
    ├── service configs
    ├── database credentials
    ├── Kafka connection
    └── encryption keys (reference to external KMS)
```

### 5.2 Local development

For local development, use Docker Compose:

```yaml
services:
  postgres:    # Single instance, all schemas
  kafka:       # Single broker (KRaft mode, no ZooKeeper)
  redis:       # Single instance
  minio:       # S3-compatible object storage
  mailhog:     # Email testing (optional, for merchant notifications)
```

All Go services run natively with `go run` or `air` (hot reload). Dashboard runs with `next dev`.

---

## 6. Cross-cutting concerns

### 6.1 Observability stack

| Layer | Tool | Purpose |
|-------|------|---------|
| Tracing | OpenTelemetry → Jaeger | Distributed request tracing |
| Metrics | Prometheus + Grafana | Latency, error rates, queue depths |
| Logging | Structured JSON → Loki/ELK | Searchable logs with correlation IDs |
| Alerting | Grafana Alerting / PagerDuty | SLO breaches, recon failures |

Every request gets a correlation ID (`X-Request-Id`) injected at the API gateway. This ID propagates through all service calls, Kafka events, and log entries. Any single payment can be traced end-to-end from creation through settlement.

### 6.2 Health checks

Every service exposes:
- `GET /healthz` — liveness (is the process running?)
- `GET /readyz` — readiness (can the service handle requests? checks DB, Kafka, Redis connections)

### 6.3 Graceful shutdown

All services handle `SIGTERM`:
1. Stop accepting new requests
2. Drain in-flight requests (30s timeout)
3. Close database connections
4. Commit Kafka consumer offsets
5. Exit

### 6.4 Configuration

Hierarchy (later overrides earlier):
1. Compiled defaults
2. YAML config file (`config.yaml`)
3. Environment variables (`PAYGATE_ORDER_SERVICE_PORT=8081`)
4. Kubernetes ConfigMaps/Secrets

Secrets (DB passwords, API keys, encryption keys) are **never** in config files. They come from environment variables injected from Kubernetes Secrets, which in production would reference an external KMS.

---

## 7. Technology decisions and rationale

| Decision | Choice | Why | Alternative considered |
|----------|--------|-----|----------------------|
| Primary language | Go | High concurrency, simple deployment, strong stdlib for HTTP/gRPC, good fit for transactional systems | Java (too heavy for portfolio), Node.js (weak concurrency model for payment workloads) |
| Database | PostgreSQL | ACID for ledger, JSONB for flexible metadata, excellent tooling, battle-tested for financial systems | MySQL (weaker JSON support), CockroachDB (overkill for portfolio) |
| Event bus | Kafka | Durable replay, consumer groups, exactly-once producer, partition ordering | RabbitMQ (no replay), Redis Streams (less ecosystem), SQS (no ordering guarantees) |
| Cache/locks | Redis | Sub-ms latency for idempotency keys, TTL-based expiry, atomic SET NX EX | Postgres advisory locks (higher latency), Memcached (no persistence) |
| Frontend | Next.js + Tailwind | SSR for dashboard, good DX, wide ecosystem | Remix (smaller community), plain React SPA (no SSR) |
| API style | REST (JSON) | Familiar to reviewers, matches Razorpay's public API style | gRPC (better for internal, worse for portfolio demos) |
| Internal comms | gRPC | Type-safe, efficient for service-to-service, streaming support | REST (higher overhead), message-only (too async for ledger writes) |
| ID generation | KSUID | Time-sortable, lexicographic, URL-safe, debuggable with prefix | UUID v4 (not sortable), Snowflake (requires coordination) |
| Containerization | Docker + K8s | Industry standard, demonstrates ops maturity | Docker Compose only (too simple for portfolio) |

---

## 8. Failure domains and blast radius

| Failure | Blast radius | Mitigation |
|---------|-------------|------------|
| PostgreSQL primary down | All writes blocked | Automatic failover to replica, connection pooling with PgBouncer retries |
| Kafka broker down | Event publishing delayed | 3-broker cluster, replication factor 2, outbox buffers events |
| Redis down | Rate limiting disabled; idempotency cache unavailable | Fail-open only for non-money-changing endpoints. For capture/refund/settlement writes, fall back to DB-backed idempotency or return 503. Never knowingly allow duplicate money movement because Redis is down. |
| Payment Service down | No new payments | HPA scales up, circuit breaker returns 503, checkout shows retry UI |
| Webhook Service down | Webhooks delayed | Events stay in Kafka until service recovers, no data loss |
| Outbox relay down | Events delayed | Events accumulate in outbox table, relay catches up on restart, alert fires if backlog > 1000 |
| Settlement batch fails | Settlements delayed | Idempotent batch — re-run safely. Alert ops. |
| Reconciliation fails | No immediate impact | Alert ops. Recon is read-only and observational. |

---

## 9. Deployment strategy

### 9.1 Rolling deployments

All stateless services use Kubernetes rolling deployments with:
- `maxUnavailable: 0` (never reduce capacity during deploy)
- `maxSurge: 1` (add one new pod before removing old)
- Readiness probe must pass before traffic routes to new pod

### 9.2 Database migrations

- Managed with `golang-migrate/migrate`
- Every migration is forward-only (no down migrations in production)
- Migrations run as a Kubernetes Job before the service deployment
- Backward-compatible schema changes only (add column, add table, add index). Never drop or rename in the same release.

### 9.3 Feature flags

For phased rollout of new features (e.g., auto-capture policy, speed refunds), use a simple feature flag table:

```sql
CREATE TABLE feature_flags (
  key TEXT PRIMARY KEY,
  enabled BOOLEAN DEFAULT false,
  merchant_ids TEXT[],  -- NULL = all merchants
  rollout_pct INT DEFAULT 0,
  updated_at TIMESTAMPTZ
);
```

Checked at request time via cached lookup (Redis, 30s TTL).

### 9.4 Extraction guardrails (advanced track)

- Never extract a synchronous money-path boundary unless:
- command idempotency table exists for the consumer
- replay and compensation are implemented and tested
- reconciliation has mismatch rules for that boundary
- runbook includes failure and recovery playbook for that boundary
