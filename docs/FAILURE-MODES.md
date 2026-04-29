# PayGate — Failure Modes and Recovery

> Every failure mode the system can encounter, what happens, and how it recovers. This is what separates a senior engineer from someone who only builds happy paths.

---

## 1. Payment path failures

### F-PAY-001: Gateway timeout during authorization

**Trigger**: Simulated gateway does not respond within 10 seconds.

**State before**: Payment attempt `status=processing`

**What happens**:
1. HTTP client timeout fires after 10s
2. Payment service cannot know if the bank authorized or not
3. Payment attempt moves to `status=failed` with `error_code=GATEWAY_TIMEOUT`
4. Order remains in `attempted` state (not `paid`)
5. No ledger entries created (nothing was captured)

**Recovery**:
- Buyer can retry on checkout (new payment attempt against same order)
- If the gateway *did* authorize (late callback arrives), the system must handle it: see F-PAY-003

**Alert**: `payment.gateway_timeout` metric increments. Alert if rate > 5% of attempts in 5 minutes.

---

### F-PAY-002: Server crash after capture DB write, before response

**Trigger**: Payment service process killed after `COMMIT` (payment → captured, outbox entry written) but before HTTP response sent.

**State before**: Payment `status=captured` in DB. Outbox entry exists. Client received no response.

**What happens**:
1. Client receives a network error (connection reset)
2. Client retries with same `Idempotency-Key`
3. Idempotency layer finds `status=in_progress` in Redis (or expired key if crash was long ago)

**Recovery path A — Redis key still exists**:
- If crash was quick and Redis key is `in_progress`: return `409 Conflict, Retry-After: 1`
- Client retries again after 1s. If the idempotency key was updated to `completed` by then, return cached response.

**Recovery path B — Redis key expired (crash lasted > 24h or Redis restarted)**:
- Client sends a new request with same idempotency key
- Idempotency SET NX succeeds (key expired)
- Service tries to execute capture again
- Capture handler checks payment state: already `captured` → return current payment (no double-capture)
- This works because the state machine rejects `captured → captured` as a no-op, not an error

**Key insight**: The state machine + idempotency key + outbox pattern together make this safe. No double-capture, no lost events.

---

### F-PAY-003: Late authorization callback

**Trigger**: Gateway returns `pending` initially. Authorization callback arrives 30 seconds later (or minutes later).

**What happens**:
1. Payment attempt `status=processing` (waiting for callback)
2. Callback arrives via webhook from gateway
3. Payment service validates callback signature
4. Transitions payment to `authorized`
5. If merchant has auto-capture, schedules capture

**Edge case**: callback arrives after the order has already expired.
- Payment service checks order status before transitioning
- If order expired → reject the late authorization, do not create a payment
- Log the event as `late_auth_rejected` for ops visibility

**Edge case**: duplicate callback (gateway sends it twice).
- Second callback tries to transition `authorized → authorized` → state machine rejects
- Return 200 to gateway (acknowledge receipt) but take no action
- Log as `duplicate_callback`

---

### F-PAY-004: Authorization succeeds, capture window expires

**Trigger**: Merchant does not capture within 5 days (configurable). Auto-capture is disabled.

**What happens**:
1. Sweeper worker runs every 5 minutes
2. Finds payments with `status=authorized AND authorized_at < NOW() - 5 days`
3. Transitions payment to `auto_refunded`
4. No ledger entries needed (nothing was captured, so no money moved)
5. Emits `payment.auto_refunded` event

**Recovery**: Merchant must create a new order and new payment. The expired authorization is gone.

---

### F-PAY-005: Partial capture amount mismatch

**Trigger**: Merchant sends capture with `amount` different from authorized amount.

**What happens**:
- If `capture_amount > authorized_amount`: reject with `BAD_REQUEST_ERROR`
- If `capture_amount < authorized_amount` and partial capture is not enabled: reject
- If `capture_amount < authorized_amount` and partial capture is enabled: capture the lesser amount, release the remainder

---

## 2. Refund failures

### F-RFN-001: Refund exceeds remaining refundable amount

**Trigger**: Multiple concurrent refund requests that individually are within limits but combined exceed the captured amount.

**What happens**:
1. Refund request A: ₹300 (captured: ₹500, refunded: ₹0) → valid
2. Refund request B: ₹300 (captured: ₹500, refunded: ₹0) → valid at validation time
3. Both proceed to create refund records

**Prevention**: Use `SELECT ... FOR UPDATE` on the payment row when validating refund eligibility. This serializes concurrent refund validations:

```sql
BEGIN;
SELECT amount, amount_refunded FROM payments WHERE id = $1 FOR UPDATE;
-- Check: amount_refunded + refund_amount <= amount
-- If valid: INSERT refund, UPDATE payments SET amount_refunded = amount_refunded + refund_amount
COMMIT;
```

---

### F-RFN-002: Gateway rejects refund

**Trigger**: Simulated gateway returns failure for refund processing.

**State**: Refund record exists with `status=processing`

**What happens**:
1. Refund moves to `status=failed`
2. Compensating ledger entry is NOT needed (the original refund ledger entry was created optimistically)
3. Actually, **design decision**: create refund ledger entries only after gateway confirms, not at creation time

**Better approach**: Refund ledger entries are created when refund moves to `processed`, not at `created`. This avoids needing compensating entries on failure.

---

## 3. Webhook delivery failures

### F-WHK-001: Merchant endpoint returns 5xx

**Trigger**: Merchant's webhook URL returns HTTP 500/502/503.

**What happens**:
1. Delivery attempt recorded with `status=failed, response_status=500`
2. Event enqueued for retry per exponential backoff schedule
3. After 18 attempts (24 hours), moved to dead-letter queue
4. `webhook.delivery.exhausted` alert fires

**Recovery**: Merchant fixes their endpoint. Ops or merchant triggers replay via `POST /v1/webhooks/{event_id}/replay`.

---

### F-WHK-002: Merchant endpoint is DNS-unreachable

**Trigger**: Domain no longer resolves.

**What happens**: Same as F-WHK-001 but `error=dns_resolution_failed` instead of HTTP status code. Retries proceed normally.

---

### F-WHK-003: Webhook delivered but merchant processes it twice

**Trigger**: Merchant's endpoint returns 200 but their processing is not idempotent. The event is replayed.

**Merchant-side problem**, but PayGate helps by:
1. Including `event_id` in every webhook payload — merchants should deduplicate by this
2. Including `created_at` timestamp — merchants can detect stale replays
3. Documentation recommends idempotent webhook handlers

---

### F-WHK-004: Webhook service is down during a payment capture

**Trigger**: Webhook service crashes or is unreachable.

**What happens**:
1. Payment capture succeeds (DB write + outbox entry committed)
2. Outbox relay publishes event to Kafka
3. Kafka retains the event (7-day retention)
4. When webhook service comes back, it resumes consuming from its last committed offset
5. All missed events are delivered (delayed, not lost)

**Key insight**: Because events go through Kafka (not direct service-to-service calls), the webhook service being down does not block payment processing or lose events.

---

## 4. Infrastructure failures

### F-INF-001: PostgreSQL primary failure

**Impact**: All write operations fail across all services.

**Mitigation**:
1. Streaming replication to standby
2. Automatic failover (Patroni/pg_auto_failover)
3. Connection pooler (PgBouncer) handles reconnection transparently
4. Services get connection errors, retry with backoff
5. Expected recovery: 15-30 seconds

**During failover**:
- Read queries can be served from replica (if read replicas are configured)
- Write queries fail with 503 → clients retry
- Outbox entries accumulate in the new primary once it's promoted
- No data loss (synchronous replication for critical schemas)

---

### F-INF-002: Kafka cluster degradation

**Impact**: Event publishing delayed, webhooks delayed, settlement processing delayed.

**Mitigation**:
1. 3-broker cluster with replication factor 2
2. If one broker fails, remaining brokers handle all partitions
3. Outbox relay retries Kafka publishes — events are buffered in PostgreSQL
4. If Kafka is fully down, outbox table acts as buffer (can handle hours of events)

**Alert**: outbox table has > 1000 unpublished entries.

---

### F-INF-003: Redis cluster failure

**Impact**: Idempotency checks, rate limiting, and webhook deduplication unavailable.

**Behavior with Redis down**:
- **Idempotency**: fail-open. Skip idempotency check, log warning. Risk of duplicate processing, but state machine prevents double-capture.
- **Rate limiting**: fail-open. Skip rate limit check. Risk of overload, but downstream services have their own connection limits.
- **Webhook dedup**: fail-open. May re-deliver a webhook. Merchants should handle duplicates.

**Recovery**: Redis restarts, all caches rebuild organically. No manual intervention needed.

---

## 5. Reconciliation failures

### F-REC-001: Missing ledger entry for captured payment

**Mismatch type**: `MISSING_LEDGER_ENTRY` (Critical)

**Root cause**: Payment was captured but ledger service call failed, and no compensating mechanism fired.

**This should be impossible** if the outbox pattern is implemented correctly (ledger write happens in the capture flow before the DB commit). If it happens, it indicates a bug in the capture flow.

**Resolution**:
1. Alert fires immediately
2. Ops investigates: check audit log for the capture event
3. If payment is legitimately captured, manually create ledger entries via admin API
4. File a bug for the capture flow

---

### F-REC-002: Settlement sum doesn't match line items

**Mismatch type**: `SETTLEMENT_SUM_MISMATCH` (Critical)

**Root cause**: Race condition during settlement batch — a refund was processed between collecting eligible payments and creating settlement items.

**Prevention**: Settlement batch runs inside a serializable transaction with `SELECT ... FOR UPDATE` on the payment rows.

**Resolution**: Void the settlement, re-run the batch.

---

### F-REC-003: Orphan settlement item

**Mismatch type**: `ORPHAN_SETTLEMENT_ITEM` (High)

**Root cause**: Payment record was deleted or modified after settlement was created. (Should not happen — payments are never deleted.)

**Resolution**: Manual investigation. The settlement item's `payment_id` should always resolve. If it doesn't, this is a data integrity issue that needs DB-level investigation.

---

## 6. Failure recovery matrix

| Failure | Detection | Auto-recovery | Manual intervention |
|---------|-----------|---------------|-------------------|
| Gateway timeout | Immediate (HTTP timeout) | Client retry with new attempt | None |
| Crash after capture write | Client retry → idempotency | State machine prevents double-capture | None |
| Late authorization | Callback arrives | Accept or reject based on order state | None |
| Capture window expiry | Sweeper worker (5 min) | Auto-refund | None |
| Concurrent refund race | DB row lock | Serialized validation | None |
| Webhook delivery failure | HTTP response code | Exponential retry, 18 attempts | Replay after dead-letter |
| Webhook service down | Health check | Kafka buffers events, auto-resume | None |
| PostgreSQL failover | Connection error | Automatic failover (15-30s) | Verify replication |
| Kafka broker failure | Producer error | Outbox buffers, auto-resume | Verify broker health |
| Redis failure | Connection error | Fail-open for all Redis uses | Restart Redis |
| Missing ledger entry | Reconciliation | ALERT (should not happen) | Manual entry creation |
| Settlement mismatch | Reconciliation | ALERT | Void and re-run batch |

---

## 7. Monitoring and alerting for failure detection

| Alert | Condition | Severity | Response |
|-------|-----------|----------|----------|
| High payment failure rate | > 10% of attempts fail in 5 min | P1 | Check gateway health |
| Outbox backlog | > 1000 unpublished entries | P2 | Check Kafka health, relay process |
| Webhook delivery exhausted | Any event reaches dead-letter | P3 | Notify merchant, check endpoint |
| Ledger imbalance | `SUM(debit) ≠ SUM(credit)` for any transaction | P1 | Halt settlements, investigate |
| Reconciliation critical mismatch | Any `MISSING_LEDGER_ENTRY` or `AMOUNT_MISMATCH` | P1 | Investigate immediately |
| Settlement batch failure | Batch job exits with error | P2 | Re-run batch |
| API latency spike | p99 > 2x target for 5 min | P2 | Check DB, Redis, service health |
| Connection pool exhaustion | Available connections < 10% | P2 | Scale service or increase pool |
