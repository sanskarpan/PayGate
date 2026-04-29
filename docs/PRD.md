# PayGate — Product Requirements Document

> A production-grade, multi-tenant payment platform inspired by Razorpay's public payment flow model.

---

## 1. Product vision

Build a payment platform that lets merchants create orders, accept payments across multiple methods, handle authorization/capture/refunds with explicit state machines, push reliable webhooks, track settlements, and reconcile every rupee end-to-end.

The goal is not a checkout demo. The goal is to demonstrate **production-grade thinking**: correctness before convenience, explicit state transitions, observability everywhere, and failure recovery as a first-class feature.

---

## 2. Success criteria

| Criteria | Target |
|----------|--------|
| Order creation p99 latency | < 50ms |
| Payment capture p99 latency | < 300ms |
| Webhook delivery (first attempt) | 99.5% within 30s |
| Webhook delivery (with retries) | 99.9% within 5 min |
| Ledger balance accuracy | Zero drift (reconciliation passes) |
| Settlement cycle | T+2 default, merchant-configurable |
| Uptime target | 99.95% (design goal) |
| RPO | < 1 minute |
| RTO | < 15 minutes |

---

## 3. User roles

### 3.1 Merchant admin
Configures business settings, API keys, webhook endpoints, settlement preferences, team access, and capture policies. Views dashboards, settlement reports, and dispute queues.

### 3.2 Merchant developer
Integrates using REST APIs and SDK. Consumes webhooks. Tests in sandbox mode. Reads API logs and delivery status.

### 3.3 Buyer / customer
Pays through hosted or embedded checkout. Sees payment confirmation. Receives refunds when issued.

### 3.4 Operations / support
Investigates payment failures, initiates refunds, views ledger entries, manages disputes, runs reconciliation, and resolves mismatches.

### 3.5 Risk / fraud analyst
Reviews flagged transactions, configures velocity rules, manages manual review queues, and applies merchant-level blocks.

### 3.6 Platform admin (internal)
Manages merchant onboarding, views system-wide metrics, configures global rate limits, rotates platform secrets, and monitors infrastructure health.

---

## 4. Product principles

1. **Every money movement is traceable** — double-entry ledger, no silent mutations.
2. **Every external call is idempotent** — retries never cause double charges.
3. **Every state transition is explicit** — no implicit jumps, no boolean flags as state.
4. **Every webhook is replayable** — delivery failures are recoverable, not lossy.
5. **Every reconciliation mismatch is explainable** — the system can tell you *why* numbers don't match, not just *that* they don't.

---

## 5. Scope by phase

### Phase 1 — Core payments backbone
- Merchant registration and API key issuance
- Order creation with amount, currency, receipt, notes
- Hosted checkout page (simulated)
- Payment callback handling and signature verification
- Payment state machine: `created → authorized → captured | failed`
- Auto-capture timeout with configurable policy
- Merchant dashboard: orders list, payment details
- Basic API authentication (key_id + key_secret)
- Request/response logging

### Phase 2 — Reliability and money movement
- Full and partial refunds as separate resources
- Refund state machine: `created → processing → processed | failed`
- Webhook delivery service with persistent queue
- Webhook retry engine (exponential backoff, max 24h)
- Webhook signature generation and verification
- Webhook replay (re-deliver any past event)
- Idempotency key support on all write endpoints
- Transactional outbox pattern for event publishing
- Settlement engine: group payments, calculate fees, track net amounts
- Settlement state machine: `created → processing → processed | failed`
- Double-entry ledger for all money movements
- Reconciliation job: payment ↔ ledger ↔ settlement

### Phase 3 — Risk and controls
- Velocity checks (per-merchant, per-IP, per-card)
- Rule-based risk scoring
- Manual review queue for flagged payments
- Per-merchant rate limits
- RBAC for merchant team members
- API key scoping (read-only, write, admin)
- Webhook secret rotation
- IP allowlisting for API access
- Audit log for every state change
- Alerting on anomalous patterns

### Phase 4 — Enterprise-grade operations
- Dispute / chargeback lifecycle
- Settlement holds and releases
- Partial settlements
- Configurable settlement cycles
- Payout workflows
- Reconciliation console with mismatch drill-down
- Event schema versioning
- Dead-letter queue dashboard
- Chaos testing harness
- Payment simulator (delayed success, late auth, timeouts)

### Phase 5 — Distributed systems maturity (advanced track)
- Ledger extraction via idempotent command API and saga orchestration
- Event schema registry with compatibility policy (`backward` default)
- Consumer contract certification before event-version rollout
- Ledger holds/reservations for disputes, risk, and payout buffers
- Payout rail simulator (bank ACH/IMPS/UPI style async lifecycle)
- DR drills with explicit RTO/RPO verification and reconciliation catch-up SLO
- Risk scoring v2: deterministic rules + model score + reason codes

---

## 6. Domain entities

| Entity | Purpose |
|--------|---------|
| `Merchant` | Business account, settings, capture policy |
| `MerchantUser` | Team member with role assignment |
| `APIKey` | Key pair (key_id + hashed secret), scoped permissions |
| `Customer` | Buyer identity, optional saved payment methods |
| `Order` | Immutable payment intent: amount, currency, receipt |
| `PaymentAttempt` | Each try against an order (may have multiple) |
| `Payment` | Successful attempt promoted to payment record |
| `Refund` | Separate from payment, with own state machine |
| `Settlement` | Grouping of captured payments for merchant payout |
| `SettlementItem` | Line item in a settlement batch |
| `LedgerEntry` | Double-entry journal: always a debit + credit pair |
| `WebhookSubscription` | Merchant's endpoint + event types + secret |
| `WebhookEvent` | Immutable event payload |
| `WebhookDeliveryAttempt` | Each HTTP POST attempt with status/response |
| `Dispute` | Chargeback/retrieval request lifecycle |
| `RiskEvent` | Fraud signal, velocity alert, or manual block |
| `AuditEvent` | Who changed what, when, from which IP |
| `ReconciliationBatch` | Snapshot of a recon run with match/mismatch counts |
| `OutboxEntry` | Transactional outbox for event relay |

---

## 7. Core flows

### 7.1 Order creation
Merchant → `POST /v1/orders` → validate amount/currency → generate `order_id` → store with `status: created` → return order object with `id`, `amount`, `currency`, `status`.

The order locks the amount. Checkout cannot tamper with it. Multiple payment attempts can reference the same order.

### 7.2 Payment initiation
Buyer opens checkout → system creates `PaymentAttempt` → attaches order, merchant, customer, method context → emits `payment.attempted` internal event. If the buyer retries, a new attempt is created against the same order. Deduplication: `order_id + idempotency_key`.

### 7.3 Authorization and capture
Gateway (simulated) returns auth success → payment moves to `authorized` → emits `payment.authorized` event. Capture is a **separate** operation: merchant calls `POST /v1/payments/{id}/capture` or auto-capture fires after configured timeout. Capture moves payment to `captured`, creates ledger entries (`Dr. Customer Receivable / Cr. Merchant Payable`), and emits `payment.captured`. If capture window expires without capture, payment moves to `auto_refunded`.

### 7.4 Refunds
Merchant calls `POST /v1/payments/{id}/refunds` with amount and reason. System validates: payment must be `captured`, refund amount ≤ remaining refundable amount. Creates a `Refund` record and emits `refund.created`. Refund processing is async: `created → processing → processed | failed`. Ledger reversal entries are created only when the gateway confirms the refund as `processed`, so failed refunds do not require compensating ledger corrections.

### 7.5 Webhook delivery
State change → outbox entry written in same transaction → relay worker reads outbox → publishes to Kafka topic → webhook delivery worker consumes event → looks up matching subscriptions → for each subscription: generate signature, POST to endpoint, record `WebhookDeliveryAttempt`. On failure: exponential backoff, max 18 retries over 24 hours. Dead-letter after exhaustion. Replay: merchant or ops can trigger re-delivery of any past event.

### 7.6 Settlements
Nightly batch: collect all `captured` payments not yet settled and older than the merchant's settlement delay → calculate platform fee per payment → compute net amount → create `Settlement` with line items → write ledger entries (`Dr. Merchant Payable / Cr. Settlement Clearing`) → emit `settlement.created`. Settlement moves through `processing → processed` after simulated bank confirmation, at which point clearing is moved to bank payout.

### 7.7 Reconciliation
Scheduled job compares: payment records ↔ ledger entries ↔ settlement items. For every captured payment, there must be a matching ledger debit/credit pair. For every settled payment, there must be a settlement line item. Mismatches are flagged with reason codes: `MISSING_LEDGER_ENTRY`, `AMOUNT_MISMATCH`, `ORPHAN_SETTLEMENT_ITEM`, `UNSETTLED_CAPTURED_PAYMENT`. Results stored in `ReconciliationBatch`.

---

## 8. Non-functional requirements

### 8.1 Performance
- Order creation: p99 < 50ms
- Payment capture: p99 < 300ms
- Refund creation: p99 < 100ms
- Dashboard page load: p95 < 2s
- Webhook first delivery: p95 < 5s

### 8.2 Reliability
- Webhook retry budget: 18 attempts over 24 hours
- Dead-letter queue for exhausted retries
- Circuit breaker on external gateway calls
- Graceful degradation: checkout works even if webhook service is down

### 8.3 Security
- API keys: HMAC-SHA256 signed, secret hashed with bcrypt at rest
- Webhook signatures: HMAC-SHA256 of raw body
- PCI posture: tokenize card data, never store CVV, CDE isolation
- Encryption: TLS 1.2+ in transit, AES-256 at rest
- RBAC: merchant admin / developer / read-only roles
- Rate limits: per-merchant, per-endpoint, burst + sustained
- Audit logging: every mutation, every auth event

### 8.4 Observability
- Structured logging (JSON) with correlation IDs
- OpenTelemetry tracing across all services
- Prometheus metrics: request rates, latencies, error rates, queue depths
- Grafana dashboards: payment funnel, webhook delivery, settlement progress
- Alerting: latency spikes, error rate thresholds, reconciliation failures

### 8.5 Data retention
- Payment records: 7 years
- Audit logs: 5 years
- Webhook delivery logs: 90 days
- Metrics: 13 months
- Reconciliation batches: 3 years
- Event schemas and compatibility audit logs: 3 years

---

## 9. Out of scope (for now)

- Real bank/PSP integrations (use simulated gateway)
- Multi-currency settlements (single currency per merchant)
- Subscription/recurring billing
- Payment links and invoices
- EMI/installment support
- Mobile SDK
- Real PCI DSS certification audit
