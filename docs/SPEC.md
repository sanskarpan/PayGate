# PayGate — Technical Specification

> Low-level technical design for every subsystem. This is the implementation contract.

---

## 1. ID generation

All public-facing IDs use a prefixed format for debuggability:

| Entity | Prefix | Example | Generator |
|--------|--------|---------|-----------|
| Order | `order_` | `order_LxR4k9mNp2vQ` | KSUID (time-sortable) |
| Payment | `pay_` | `pay_Mn3qR7sWx1yZ` | KSUID |
| Refund | `rfnd_` | `rfnd_Kp5tV2wXz8aB` | KSUID |
| Settlement | `sttl_` | `sttl_Jq8uY3xAb5cD` | KSUID |
| Webhook Event | `evt_` | `evt_Hs7vZ4yBc6dE` | KSUID |
| Merchant | `merch_` | `merch_Gt6wA5zCd7eF` | KSUID |
| API Key ID | `rzp_test_` / `rzp_live_` | `rzp_test_Fs5xB6aDe8fG` | KSUID |
| Idempotency | — | Client-provided, max 64 chars | UUID v4 recommended |
| Ledger Entry | `le_` | `le_Er4yC7bEf9gH` | KSUID |
| Dispute | `disp_` | `disp_Dq3zD8cFg0hI` | KSUID |

KSUID chosen over UUID v4 because: time-sortable (good for cursor pagination), lexicographically orderable, and 27-char base62 encoding is URL-safe.

---

## 2. State machines

### 2.1 Order states

```
created ──→ attempted ──→ paid
   │            │           │
   │            ▼           │
   │         failed         │
   ▼                        ▼
 expired               (terminal)
```

| From | To | Trigger |
|------|----|---------|
| `created` | `attempted` | First payment attempt received |
| `created` | `expired` | No attempt within 30 min (configurable) |
| `attempted` | `paid` | Payment captured successfully |
| `attempted` | `failed` | All attempts failed |
| `paid` | — | Terminal state |
| `expired` | — | Terminal state |
| `failed` | — | Terminal state |

### 2.2 Payment states

```
created ──→ authorized ──→ captured
   │            │              │
   ▼            ▼              │
 failed    auto_refunded       │
                               ▼
                          (terminal — eligible for refund)
```

| From | To | Trigger |
|------|----|---------|
| `created` | `authorized` | Gateway returns auth success |
| `created` | `failed` | Gateway returns auth failure or timeout |
| `authorized` | `captured` | Merchant captures or auto-capture fires |
| `authorized` | `auto_refunded` | Capture window expires (configurable, default 5 days) |
| `captured` | — | Terminal. Refunds are separate records. |
| `failed` | — | Terminal |
| `auto_refunded` | — | Terminal |

**Auto-capture logic**: If merchant has `auto_capture: true` (or delay in seconds), a delayed job fires capture after the delay. If no capture and the auth window expires, the system auto-refunds. This is checked by a periodic sweeper worker.

### 2.3 Refund states

```
created ──→ processing ──→ processed
                │
                ▼
             failed
```

| From | To | Trigger |
|------|----|---------|
| `created` | `processing` | Refund job picks up from queue |
| `processing` | `processed` | Gateway confirms refund |
| `processing` | `failed` | Gateway rejects refund or timeout |

### 2.4 Settlement states

```
created ──→ processing ──→ processed
                │
                ▼
             failed ──→ retrying ──→ processing
```

### 2.5 Webhook delivery states

```
pending ──→ delivered
   │
   ▼
retrying ──→ delivered
   │
   ▼
dead_lettered
```

---

## 3. API contracts

### 3.1 Authentication

All API requests use HTTP Basic Auth: `key_id:key_secret`. The `key_id` is the public identifier; the `key_secret` is shown once at creation and stored as a bcrypt hash.

```
Authorization: Basic base64(rzp_test_Fs5xB6aDe8fG:secret_value)
```

Rate limits are enforced per `key_id`:
- Default: 25 requests/second burst, 10 requests/second sustained
- Configurable per merchant

### 3.2 Idempotency

All `POST` endpoints accept an `Idempotency-Key` header. Behavior:

1. On first request: execute normally, cache response keyed by `merchant_id:endpoint:idempotency_key` in Redis with `SET NX EX 86400` (24h TTL). For money-changing endpoints, also persist an idempotency record in Postgres inside the business transaction.
2. On duplicate request (key exists): return cached response with `Idempotent-Replayed: true` header. Do not re-execute.
3. On in-flight duplicate (key exists but no cached response yet): return `409 Conflict` with `Retry-After: 1`.

Key format stored in Redis: `idempotency:{merchant_id}:{endpoint_hash}:{client_key}`

### 3.3 Core endpoints

#### Create order
```
POST /v1/orders
Content-Type: application/json
Authorization: Basic {credentials}
Idempotency-Key: {uuid}

Request:
{
  "amount": 50000,           // in smallest currency unit (paise)
  "currency": "INR",
  "receipt": "rcpt_2024_001",
  "notes": {
    "policy_id": "POL-123"
  },
  "partial_payment": false
}

Response: 201 Created
{
  "id": "order_LxR4k9mNp2vQ",
  "entity": "order",
  "amount": 50000,
  "amount_paid": 0,
  "amount_due": 50000,
  "currency": "INR",
  "receipt": "rcpt_2024_001",
  "status": "created",
  "notes": { "policy_id": "POL-123" },
  "created_at": 1714000000
}
```

#### Capture payment
```
POST /v1/payments/{payment_id}/capture
Authorization: Basic {credentials}
Idempotency-Key: {uuid}

Request:
{
  "amount": 50000,
  "currency": "INR"
}

Response: 200 OK
{
  "id": "pay_Mn3qR7sWx1yZ",
  "entity": "payment",
  "amount": 50000,
  "currency": "INR",
  "status": "captured",
  "order_id": "order_LxR4k9mNp2vQ",
  "method": "card",
  "captured": true,
  "captured_at": 1714000100,
  "created_at": 1714000050
}
```

#### Create refund
```
POST /v1/payments/{payment_id}/refunds
Authorization: Basic {credentials}
Idempotency-Key: {uuid}

Request:
{
  "amount": 25000,         // partial refund
  "speed": "normal",       // or "optimum"
  "notes": {
    "reason": "customer_request"
  },
  "receipt": "rfnd_rcpt_001"
}

Response: 201 Created
{
  "id": "rfnd_Kp5tV2wXz8aB",
  "entity": "refund",
  "payment_id": "pay_Mn3qR7sWx1yZ",
  "amount": 25000,
  "currency": "INR",
  "status": "created",
  "speed_requested": "normal",
  "receipt": "rfnd_rcpt_001",
  "created_at": 1714001000
}
```

#### List payments (with cursor pagination)
```
GET /v1/payments?count=10&from=1714000000&to=1714100000&cursor={opaque_cursor}
Authorization: Basic {credentials}

Response: 200 OK
{
  "entity": "collection",
  "count": 10,
  "items": [ ... ],
  "has_more": true
}
```

#### Webhook event payload
```json
{
  "entity": "event",
  "event_id": "evt_Hs7vZ4yBc6dE",
  "event": "payment.captured",
  "account_id": "merch_Gt6wA5zCd7eF",
  "contains": ["payment"],
  "payload": {
    "payment": {
      "entity": {
        "id": "pay_Mn3qR7sWx1yZ",
        "amount": 50000,
        "currency": "INR",
        "status": "captured",
        "order_id": "order_LxR4k9mNp2vQ",
        "method": "card"
      }
    }
  },
  "created_at": 1714000100
}
```

### 3.4 Webhook signature verification

```
X-PayGate-Signature: HMAC-SHA256(webhook_secret, raw_request_body)
```

Merchant must verify using the **raw** request body (not parsed JSON), because JSON parsing can reorder keys or alter whitespace. The webhook secret is per-subscription and rotatable.

### 3.5 Error response format
```json
{
  "error": {
    "code": "BAD_REQUEST_ERROR",
    "description": "The amount field is required",
    "field": "amount",
    "source": "business",
    "step": "payment_initiation",
    "reason": "input_validation_failed",
    "metadata": {}
  }
}
```

Standard error codes: `BAD_REQUEST_ERROR`, `GATEWAY_ERROR`, `SERVER_ERROR`, `UNAUTHORIZED`, `RATE_LIMITED`, `IDEMPOTENCY_CONFLICT`.

---

## 4. Double-entry ledger

### 4.1 Ledger accounts

| Account | Type | Description |
|---------|------|-------------|
| `CUSTOMER_RECEIVABLE` | Asset | Money owed by customer |
| `MERCHANT_PAYABLE` | Liability | Money owed to merchant |
| `PLATFORM_FEE_REVENUE` | Revenue | Platform commission |
| `MERCHANT_BANK_PAYOUT` | Asset | Money sent to merchant bank |
| `REFUND_CLEARING` | Liability | Pending refund to customer |
| `TAX_PAYABLE` | Liability | GST/tax on platform fees |
| `SETTLEMENT_CLEARING` | Liability | In-transit settlement |

### 4.2 Journal entries per flow

**Payment captured (₹500, 2% platform fee):**
```
Dr. CUSTOMER_RECEIVABLE    ₹500.00
  Cr. MERCHANT_PAYABLE              ₹490.00
  Cr. PLATFORM_FEE_REVENUE          ₹10.00
```

**Full refund confirmed by gateway (₹500):**
```
Dr. MERCHANT_PAYABLE       ₹490.00
Dr. PLATFORM_FEE_REVENUE   ₹10.00
  Cr. REFUND_CLEARING               ₹500.00
```

**Partial refund confirmed by gateway (₹200 of ₹500):**
```
Dr. MERCHANT_PAYABLE       ₹196.00
Dr. PLATFORM_FEE_REVENUE   ₹4.00
  Cr. REFUND_CLEARING               ₹200.00
```

**Settlement payout (₹490 net to merchant):**
```
Dr. MERCHANT_PAYABLE       ₹490.00
  Cr. SETTLEMENT_CLEARING           ₹490.00
```

**Settlement confirmed:**
```
Dr. SETTLEMENT_CLEARING    ₹490.00
  Cr. MERCHANT_BANK_PAYOUT          ₹490.00
```

### 4.3 Ledger rules

1. Every ledger transaction must balance: `SUM(debit_amount) = SUM(credit_amount)` across all rows with the same `transaction_id`.
2. Every entry references its source: `source_type` (payment, refund, settlement) + `source_id`.
3. Entries are **append-only**. Corrections are compensating entries, not updates.
4. A periodic ledger-balance job sums all debits and credits per account and asserts zero net (assets - liabilities - equity = 0).
5. Capture, refund-confirmation, settlement-creation, audit, and outbox writes must share one PostgreSQL transaction in Phase 1. If Ledger is extracted later, use an explicit saga with idempotent ledger commands.

---

## 5. Outbox pattern

### 5.1 Why

The system must update the database (e.g., move payment to `captured`) **and** publish an event (e.g., `payment.captured` to Kafka) atomically. Without an outbox, either:
- Write to DB succeeds but Kafka publish fails → event lost
- Kafka publish succeeds but DB write fails → phantom event

### 5.2 How

1. In the same Postgres transaction that updates the payment state, insert a row into the `outbox` table:
   ```sql
   INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload, created_at)
   VALUES ('evt_...', 'payment', 'pay_...', 'payment.captured', '{"..."}', NOW());
   ```
2. A **relay worker** (separate process) polls the outbox table:
   ```sql
   SELECT * FROM outbox WHERE published_at IS NULL ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED;
   ```
3. For each row, publish to Kafka topic. On success, mark:
   ```sql
   UPDATE outbox SET published_at = NOW() WHERE id = $1;
   ```
4. Delivery guarantee: **at-least-once**. Consumers must be idempotent (deduplicate by `event_id`).
5. Cleanup: a nightly job deletes outbox entries older than 7 days where `published_at IS NOT NULL`.

### 5.3 Relay worker details

- Polling interval: 100ms
- Batch size: 100
- On Kafka publish failure: retry 3 times with 1s backoff, then skip (row stays in outbox for next poll)
- Health check: if relay falls behind by > 1000 unpublished entries, fire alert
- Kafka topic partitioning: by `merchant_id` to preserve per-merchant ordering

---

## 6. Webhook delivery engine

### 6.1 Architecture

```
Kafka consumer (webhook.events topic)
  → Look up matching WebhookSubscriptions
  → For each subscription:
      → Generate signature: HMAC-SHA256(secret, raw_payload)
      → HTTP POST to endpoint
      → Record WebhookDeliveryAttempt
      → On failure: enqueue retry
```

### 6.2 Retry policy

| Attempt | Delay | Cumulative |
|---------|-------|------------|
| 1 | immediate | 0s |
| 2 | 5s | 5s |
| 3 | 30s | 35s |
| 4 | 2 min | ~2.5 min |
| 5 | 10 min | ~12.5 min |
| 6 | 30 min | ~42.5 min |
| 7 | 1 hour | ~1.7 hrs |
| 8 | 2 hours | ~3.7 hrs |
| 9–18 | 2 hours each | ~21.7 hrs |

After 18 failed attempts (~24 hours), move to dead-letter queue. Emit `webhook.delivery.exhausted` internal alert.

### 6.3 Delivery attempt record

```json
{
  "id": "wda_...",
  "webhook_event_id": "evt_...",
  "subscription_id": "wsub_...",
  "attempt_number": 3,
  "request_url": "https://merchant.com/webhooks",
  "request_headers": { "X-PayGate-Signature": "..." },
  "response_status": 500,
  "response_body_truncated": "Internal Server Error",
  "response_time_ms": 2340,
  "error": null,
  "created_at": 1714002000
}
```

### 6.4 Duplicate suppression

Each event+subscription combination has a delivery fingerprint: `SHA256(event_id + subscription_id)`. Before posting, check if fingerprint exists in Redis with a 48h TTL. If exists and previous delivery was `2xx`, skip. This prevents re-delivery on relay restarts.

### 6.5 Replay

`POST /v1/webhooks/events/{event_id}/replay` — re-enqueues the event for delivery to all matching subscriptions. Bypasses duplicate suppression. Available to merchant admin and ops.

---

## 7. Settlement engine

### 7.1 Settlement cycle

Default: T+2 (configurable per merchant). Settlement runs as a nightly batch job. The PRD success target is T+2 unless a merchant setting overrides it.

### 7.2 Settlement batch process

1. **Collect eligible payments**: `status = captured AND settled = false AND captured_at < (now - settlement_delay)`
2. **Group by merchant**: one settlement per merchant per cycle
3. **Calculate fees**: per-payment platform fee (configurable rate, default 2%)
4. **Compute net amount**: `sum(captured_amounts) - sum(platform_fees) - sum(refunded_amounts_since_last_settlement)`
5. **Create settlement record** with line items
6. **Write ledger entries** for each line item
7. **Mark payments** as `settled = true` with `settlement_id`
8. **Emit** `settlement.created` event

### 7.3 Settlement holds

Operations can place a hold on a merchant's settlements. While held:
- Payments continue to be captured normally
- Settlement batch skips the merchant
- Hold reason and timestamp are recorded
- Release requires ops approval

---

## 8. Reconciliation engine

### 8.1 Three-way reconciliation

```
Payment records ←→ Ledger entries ←→ Settlement items
```

For every captured payment:
1. There must exist exactly one debit/credit ledger pair with `source_type=payment, source_id={payment_id}`
2. If settled, there must exist a settlement item with `payment_id={payment_id}` and matching net amount

### 8.2 Mismatch types

| Code | Description | Severity |
|------|-------------|----------|
| `MISSING_LEDGER_ENTRY` | Captured payment has no ledger entry | Critical |
| `AMOUNT_MISMATCH` | Ledger amount ≠ payment amount | Critical |
| `ORPHAN_SETTLEMENT_ITEM` | Settlement item references non-existent payment | High |
| `UNSETTLED_PAST_DUE` | Captured payment past settlement window, not settled | Medium |
| `DUPLICATE_LEDGER_ENTRY` | Multiple ledger pairs for same payment | Critical |
| `REFUND_LEDGER_MISMATCH` | Refund record amount ≠ ledger reversal amount | Critical |
| `SETTLEMENT_SUM_MISMATCH` | Settlement total ≠ sum of line items | Critical |

### 8.3 Reconciliation schedule

- **Continuous**: ledger balance check (sum of all debits = sum of all credits) runs every 5 minutes
- **Hourly**: payment-to-ledger reconciliation for payments captured in last 2 hours
- **Nightly**: full three-way reconciliation for all unsettled payments
- **Weekly**: historical reconciliation for last 30 days (catch delayed corrections)

---

## 9. Idempotency implementation

### 9.1 Storage

Redis key: `idempotency:{merchant_id}:{endpoint_hash}:{client_key}`
Value: JSON `{ "status": "completed|in_progress", "response_code": 201, "response_body": "..." }`
TTL: 24 hours

Postgres table for money-changing endpoints:
`idempotency_records(merchant_id, endpoint_hash, client_key, request_hash, status, resource_type, resource_id, response_code, response_body, expires_at)`.

Redis is an acceleration layer, not the source of truth for capture/refund/settlement idempotency.

### 9.2 Flow

```
1. Request arrives with Idempotency-Key header
2. Compute Redis key
3. For money-changing endpoints, insert or lock the Postgres idempotency row first. For low-risk endpoints, use Redis only.
4. SET NX EX 86400 with status=in_progress as a cache hint
5. If the key is new:
   a. Execute request
   b. Store response/resource pointer in Postgres and Redis
   c. Return response
6. If the key exists:
   a. GET the key
   b. If status=completed: return cached response + Idempotent-Replayed: true header
   c. If status=in_progress: return 409 Conflict + Retry-After: 1
```

### 9.3 Edge cases

- If the server crashes after setting Redis but before execution: Redis eventually expires. Money-changing endpoints recover from the Postgres idempotency record, not from Redis alone.
- If the same idempotency key is reused with a different request body: return `409 IDEMPOTENCY_CONFLICT` and do not execute.
- If the response is too large for Redis (>1MB): store only the response code and resource ID. Client can GET the resource.

---

## 10. Simulated payment gateway

Since this is a portfolio project, the "bank/PSP" is simulated. The gateway service supports configurable behaviors:

| Scenario | Config | Behavior |
|----------|--------|----------|
| Happy path | `mode: success` | Auth succeeds in 100ms |
| Slow auth | `mode: slow` | Auth succeeds after 3-5s delay |
| Late auth | `mode: late_callback` | Auth returns pending, callback arrives 30s later |
| Intermittent failure | `mode: flaky, failure_rate: 0.3` | 30% of auths fail randomly |
| Gateway timeout | `mode: timeout` | No response for 30s |
| Duplicate callback | `mode: duplicate` | Success callback sent twice |
| Partial auth | `mode: partial` | Auth for less than requested amount |

This lets you demonstrate and test every failure recovery path.

---

## 11. Event catalog

| Event | Emitted when | Kafka topic |
|-------|-------------|-------------|
| `order.created` | Order created | `paygate.orders` |
| `order.paid` | Payment captured for order | `paygate.orders` |
| `payment.authorized` | Gateway returns auth success | `paygate.payments` |
| `payment.captured` | Capture succeeds | `paygate.payments` |
| `payment.failed` | Auth or capture fails | `paygate.payments` |
| `refund.created` | Refund record created | `paygate.refunds` |
| `refund.processed` | Refund confirmed | `paygate.refunds` |
| `refund.failed` | Refund rejected | `paygate.refunds` |
| `settlement.created` | Settlement batch created | `paygate.settlements` |
| `settlement.processed` | Settlement confirmed | `paygate.settlements` |
| `webhook.delivery.failed` | Delivery attempt failed | `paygate.internal` |
| `webhook.delivery.exhausted` | All retries exhausted | `paygate.internal` |
| `recon.mismatch.detected` | Reconciliation found mismatch | `paygate.internal` |

### 11.1 Event schema versioning

Every event includes a `schema_version` field (semver). Consumers must handle unknown fields gracefully. Breaking changes increment the major version and are published to a new topic partition or topic suffix (e.g., `paygate.payments.v2`). Old consumers continue reading the v1 stream until migrated.

---

## 12. Security design

### 12.1 Cardholder data environment (CDE) boundary

```
┌─────────────────────────────────────────┐
│ CDE (isolated network segment)          │
│  ┌──────────────┐  ┌─────────────────┐  │
│  │ Tokenization  │  │ Gateway proxy   │  │
│  │ vault         │  │ (simulated)     │  │
│  └──────────────┘  └─────────────────┘  │
└─────────────────────────────────────────┘
         │ tokens only            │ tokens only
┌─────────────────────────────────────────┐
│ Non-CDE services                        │
│  Order, Payment, Refund, Settlement,    │
│  Webhook, Reconciliation, Dashboard     │
└─────────────────────────────────────────┘
```

Card numbers never leave the CDE. Non-CDE services only see tokens like `tok_xxxx1234`. The tokenization vault:
- Accepts raw card data from checkout
- Returns a token
- Only the gateway proxy can de-tokenize (to send to the bank)
- Vault data encrypted with AES-256-GCM, key in KMS
- CVV is used for auth but **never stored** (not even encrypted)

### 12.2 API key lifecycle

1. Merchant admin generates key pair via dashboard
2. `key_secret` displayed once; stored as `bcrypt(key_secret)` in DB
3. `key_id` is the public identifier
4. Keys support scoping: `read`, `write`, `admin`
5. Keys are rotatable: create new pair, migrate, revoke old pair
6. Revoked keys return `401` immediately

### 12.3 Request scrubbing

All request/response logging passes through a scrubber that removes: card numbers, CVV, key_secret values, webhook secrets, and any field matching `password|secret|token|cvv|card_number` regex. Scrubbing happens **before** the log is written, not after.

---

## 13. Advanced distributed track (optional, higher complexity)

### 13.1 Saga orchestration for extracted services

When Payment, Ledger, and Settlement are independently deployed, use a saga with idempotent commands:

1. `payment.capture.requested`
2. `ledger.capture.post.requested`
3. `ledger.capture.posted` (or `ledger.capture.rejected`)
4. `payment.capture.committed` (or `payment.capture.failed`)
5. `settlement.eligibility.updated`

Rules:
- Every saga step carries `saga_id`, `step_id`, and `idempotency_key`.
- Every command consumer stores `processed_commands(command_id)` and must be idempotent.
- Compensation is explicit and append-only (never delete prior money records).

### 13.2 Event schema governance

- Every emitted event references a schema ID and version.
- Compatibility policy: `backward` for additive changes, `major` bump for breaking changes.
- CI gate must run producer schema checks and consumer contract tests before merge.
- Rollout policy: dual-publish (`v1` + `v2`) until all critical consumers certify.

### 13.3 Ledger reservations and holds

Support pre-commit holds for risk/dispute scenarios:
- `hold.created` reserves a liability amount
- `hold.released` removes reservation
- `hold.committed` converts hold into final posting

This enables delayed capture/payout flows without violating balance invariants.

### 13.4 Disaster recovery verification

- Quarterly DR drill: fail primary region and recover within target RTO/RPO.
- Post-recovery reconciliation must complete and show zero critical mismatches before normal settlement resumes.
