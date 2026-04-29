# PayGate — Data Flow

> End-to-end data flow for every core operation. Trace the path of every request, event, and mutation.

---

## 1. Payment lifecycle — complete flow

This is the full happy-path data flow from order creation through settlement.

```
MERCHANT                    API GATEWAY                ORDER SERVICE
   │                            │                           │
   │  POST /v1/orders           │                           │
   ├───────────────────────────►│  auth, rate limit,        │
   │                            │  inject correlation ID    │
   │                            ├──────────────────────────►│
   │                            │                           │  validate amount/currency
   │                            │                           │  generate order_id (KSUID)
   │                            │                           │  INSERT orders (status=created)
   │                            │                           │  INSERT outbox (order.created)
   │                            │                           │  COMMIT transaction
   │                            │◄──────────────────────────┤
   │◄───────────────────────────┤  201 Created              │
   │  { id: order_xxx }         │                           │
```

```
BUYER (CHECKOUT)            API GATEWAY                PAYMENT SERVICE          GATEWAY PROXY
   │                            │                           │                       │
   │  Submit payment            │                           │                       │
   ├───────────────────────────►│                           │                       │
   │                            ├──────────────────────────►│                       │
   │                            │                           │  validate order exists │
   │                            │                           │  check idempotency key│
   │                            │                           │  create PaymentAttempt│
   │                            │                           │  call gateway         │
   │                            │                           ├──────────────────────►│
   │                            │                           │                       │  simulate auth
   │                            │                           │◄──────────────────────┤
   │                            │                           │  auth_code received   │
   │                            │                           │                       │
   │                            │                           │  BEGIN TRANSACTION:
   │                            │                           │    UPDATE payment → authorized
   │                            │                           │    INSERT outbox (payment.authorized)
   │                            │                           │  COMMIT
   │                            │                           │
   │                            │◄──────────────────────────┤
   │◄───────────────────────────┤  payment.authorized       │
```

```
MERCHANT                    API GATEWAY                PAYMENT SERVICE          LEDGER MODULE
   │                            │                           │                       │
   │  POST /payments/x/capture  │                           │                       │
   ├───────────────────────────►│                           │                       │
   │                            ├──────────────────────────►│                       │
   │                            │                           │  validate state=authorized
   │                            │                           │  check idempotency    │
   │                            │                           │                       │
   │                            │                           │  BEGIN TRANSACTION:
   │                            │                           │    lock payment row
   │                            │                           │    validate transition
   │                            │                           │    validate ledger balances
   │                            │                           │    INSERT ledger_transaction
   │                            │                           │    INSERT ledger_entries
   │                            │                           │      Dr. CUST_RECEIVABLE
   │                            │                           │      Cr. MERCH_PAYABLE
   │                            │                           │      Cr. PLATFORM_FEE
   │                            │                           │    UPDATE payment → captured
   │                            │                           │    INSERT audit_event
   │                            │                           │    INSERT outbox (payment.captured)
   │                            │                           │  COMMIT
   │                            │                           │
   │                            │◄──────────────────────────┤
   │◄───────────────────────────┤  200 OK (captured)        │
```

---

## 2. Event propagation flow

```
PAYMENT SERVICE    →    OUTBOX TABLE    →    OUTBOX RELAY    →    KAFKA
      │                      │                    │                  │
      │  INSERT outbox       │                    │                  │
      │  (same txn as        │                    │                  │
      │   state change)      │                    │                  │
      │─────────────────────►│                    │                  │
      │                      │                    │                  │
      │                      │  poll every 100ms  │                  │
      │                      │◄───────────────────┤                  │
      │                      │  SELECT ... FOR    │                  │
      │                      │  UPDATE SKIP LOCKED│                  │
      │                      │───────────────────►│                  │
      │                      │                    │  produce to      │
      │                      │                    │  paygate.payments│
      │                      │                    ├─────────────────►│
      │                      │                    │                  │
      │                      │                    │  on ack:         │
      │                      │  UPDATE            │  mark published  │
      │                      │◄───────────────────┤                  │
      │                      │  published_at=NOW()│                  │
```

```
KAFKA              →    WEBHOOK SERVICE    →    MERCHANT ENDPOINT
  │                          │                        │
  │  consume event           │                        │
  ├─────────────────────────►│                        │
  │                          │  match subscriptions   │
  │                          │  check dedup (Redis)   │
  │                          │  generate signature:   │
  │                          │    HMAC-SHA256(secret,  │
  │                          │    raw_body)           │
  │                          │                        │
  │                          │  HTTP POST             │
  │                          ├───────────────────────►│
  │                          │                        │  verify signature
  │                          │                        │  process event
  │                          │◄───────────────────────┤
  │                          │  200 OK                │
  │                          │                        │
  │                          │  record delivery       │
  │                          │  attempt (success)     │
  │                          │  set dedup key (Redis) │
```

---

## 3. Refund data flow

```
MERCHANT                    REFUND SERVICE             LEDGER MODULE
   │                            │                           │
   │  POST /payments/x/refunds  │                           │
   ├───────────────────────────►│                           │
   │                            │  validate:                │
   │                            │    payment.status=captured│
   │                            │    amount ≤ remaining     │
   │                            │    check idempotency      │
   │                            │                           │
   │                            │  BEGIN TRANSACTION:
   │                            │    lock payment row
   │                            │    INSERT refund (status=created)
   │                            │    UPDATE payment.amount_refunded_pending
   │                            │    INSERT outbox (refund.created)
   │                            │  COMMIT
   │                            │
   │◄──────────────────────────┤  201 Created               │
   │                            │                           │
   │                            │  ─── async processing ──► │
   │                            │  queue picks up refund    │
   │                            │  call gateway for refund  │
   │                            │  BEGIN TRANSACTION:       │
   │                            │    INSERT ledger reversal │
   │                            │      Dr. MERCH_PAYABLE    │
   │                            │      Dr. PLATFORM_FEE     │
   │                            │      Cr. REFUND_CLEARING  │
   │                            │    UPDATE refund → processed
   │                            │    INSERT outbox          │
   │                            │      (refund.processed)   │
   │                            │  COMMIT                   │
```

---

## 4. Settlement data flow

```
CRON TRIGGER            SETTLEMENT SERVICE          LEDGER MODULE           PAYMENTS TABLE
   │                         │                           │                       │
   │  trigger batch          │                           │                       │
   ├────────────────────────►│                           │                       │
   │                         │  query eligible payments  │                       │
   │                         ├──────────────────────────────────────────────────►│
   │                         │  SELECT * FROM payments   │                       │
   │                         │  WHERE status=captured    │                       │
   │                         │    AND settled=false      │                       │
   │                         │    AND captured_at < cutoff│                      │
   │                         │◄──────────────────────────────────────────────────┤
   │                         │                           │                       │
   │                         │  for each merchant:       │                       │
   │                         │    calculate fees         │                       │
   │                         │    compute net amount     │                       │
   │                         │                           │                       │
   │                         │  BEGIN TRANSACTION:
   │                         │    lock eligible payments │                       │
   │                         │    INSERT settlement (status=created)
   │                         │    INSERT settlement_items
   │                         │    INSERT settlement ledger entries
   │                         │      Dr. MERCH_PAYABLE
   │                         │      Cr. SETTLEMENT_CLR
   │                         │    UPDATE payments SET settled=true
   │                         │    INSERT outbox (settlement.created)
   │                         │  COMMIT
```

---

## 5. Reconciliation data flow

```
CRON TRIGGER            RECON WORKER               PAYMENTS DB      LEDGER DB       SETTLEMENTS DB
   │                         │                         │                │                │
   │  trigger recon          │                         │                │                │
   ├────────────────────────►│                         │                │                │
   │                         │  fetch captured payments│                │                │
   │                         ├────────────────────────►│                │                │
   │                         │◄────────────────────────┤                │                │
   │                         │                         │                │                │
   │                         │  fetch ledger entries   │                │                │
   │                         ├─────────────────────────────────────────►│                │
   │                         │◄─────────────────────────────────────────┤                │
   │                         │                         │                │                │
   │                         │  fetch settlement items │                │                │
   │                         ├──────────────────────────────────────────────────────────►│
   │                         │◄──────────────────────────────────────────────────────────┤
   │                         │                         │                │                │
   │                         │  THREE-WAY MATCH:
   │                         │  for each payment:
   │                         │    1. find ledger pair → missing? MISSING_LEDGER_ENTRY
   │                         │    2. amounts match?   → no?      AMOUNT_MISMATCH
   │                         │    3. if settled, find  settlement item
   │                         │       → missing?                  ORPHAN or UNSETTLED
   │                         │
   │                         │  for each settlement item:
   │                         │    1. find payment     → missing? ORPHAN_SETTLEMENT_ITEM
   │                         │
   │                         │  store ReconciliationBatch
   │                         │  emit alerts for critical mismatches
```

---

## 6. Idempotency flow

```
REQUEST ARRIVES          API GATEWAY              SERVICE                    REDIS
    │                        │                        │                        │
    │  POST with             │                        │                        │
    │  Idempotency-Key: abc  │                        │                        │
    ├───────────────────────►│                        │                        │
    │                        ├───────────────────────►│                        │
    │                        │                        │  compute Redis key     │
    │                        │                        │  SET NX EX 86400      │
    │                        │                        ├───────────────────────►│
    │                        │                        │                        │
    │                        │   CASE 1: Key is new (SET succeeded)            │
    │                        │                        │◄───────────────────────┤
    │                        │                        │  OK (key set)          │
    │                        │                        │  execute business logic│
    │                        │                        │  store response in key │
    │                        │                        ├───────────────────────►│
    │                        │◄───────────────────────┤                        │
    │◄───────────────────────┤  200/201 (normal)      │                        │
    │                        │                        │                        │
    │                        │   CASE 2: Key exists, completed                 │
    │                        │                        │◄───────────────────────┤
    │                        │                        │  { status: completed } │
    │                        │◄───────────────────────┤                        │
    │◄───────────────────────┤  200/201 + header:     │                        │
    │                        │  Idempotent-Replayed   │                        │
    │                        │                        │                        │
    │                        │   CASE 3: Key exists, in_progress               │
    │                        │                        │◄───────────────────────┤
    │                        │                        │  { status: in_progress}│
    │                        │◄───────────────────────┤                        │
    │◄───────────────────────┤  409 Conflict          │                        │
    │                        │  Retry-After: 1        │                        │
```

---

## 7. Auto-capture flow

```
PAYMENT CAPTURED          PAYMENT SERVICE          REDIS (TIMER)           SWEEPER WORKER
(status=authorized)            │                        │                       │
       │                       │                        │                       │
       │  on authorization:    │                        │                       │
       │  if merchant has      │                        │                       │
       │  auto_capture config  │                        │                       │
       │                       │  SET autocapture:{id}  │                       │
       │                       │  EX {delay_seconds}    │                       │
       │                       ├───────────────────────►│                       │
       │                       │                        │                       │
       │                       │                        │  (key expires)        │
       │                       │                        │                       │
       │                       │                        │    (alternative path) │
       │                       │                        │                       │
       │  sweeper runs every 5 min:                     │                       │
       │                       │                        │◄──────────────────────┤
       │                       │  SELECT payments       │  query authorized    │
       │                       │  WHERE status=authorized│  payments past      │
       │                       │  AND authorized_at <   │  auto-capture window │
       │                       │  NOW() - delay         │                       │
       │                       │                        │                       │
       │                       │  for each: call capture│  internally           │
       │                       │  (same flow as manual  │  capture)             │
       │                       │                        │                       │
       │  if auth window expired (5 days default):      │                       │
       │                       │  UPDATE → auto_refunded│                       │
       │                       │  INSERT outbox          │                       │
```

---

## 8. Webhook retry flow

```
INITIAL DELIVERY FAILS      WEBHOOK SERVICE           REDIS (RETRY QUEUE)      RETRY WORKER
       │                          │                        │                       │
       │  HTTP POST → 5xx        │                        │                       │
       │                          │  record failed attempt │                       │
       │                          │  compute next retry    │                       │
       │                          │  (exponential backoff) │                       │
       │                          │                        │                       │
       │                          │  ZADD retry_queue      │                       │
       │                          │  score=next_retry_time │                       │
       │                          ├───────────────────────►│                       │
       │                          │                        │                       │
       │                          │                        │  (time passes)        │
       │                          │                        │                       │
       │                          │                        │◄──────────────────────┤
       │                          │                        │  ZRANGEBYSCORE        │
       │                          │                        │  0, NOW(), LIMIT 100  │
       │                          │                        ├──────────────────────►│
       │                          │                        │                       │
       │                          │◄────────────────────────────────────────────────┤
       │                          │  retry delivery        │                       │
       │                          │  attempt N+1           │                       │
       │                          │                        │                       │
       │  if attempt 18 fails:    │                        │                       │
       │                          │  move to dead-letter   │                       │
       │                          │  topic in Kafka        │                       │
       │                          │  emit alert            │                       │
```

---

## 9. Request tracing — full example

A single capture request generates this trace:

```
Trace ID: abc-123-def-456

Span 1: API Gateway
  ├── duration: 2ms
  ├── operation: authenticate_request
  ├── merchant_id: merch_xxx
  └── rate_limit: pass

Span 2: Payment Service → validate
  ├── duration: 5ms
  ├── operation: validate_capture_request
  ├── payment_id: pay_xxx
  └── current_state: authorized

Span 3: Payment Service → Ledger Module
  ├── duration: 12ms
  ├── operation: create_journal_entries_in_transaction
  ├── entries: 3 (debit + 2 credits)
  └── total_amount: 50000

Span 4: Payment Service → PostgreSQL
  ├── duration: 8ms
  ├── operation: update_payment_state + insert_outbox
  └── transaction: committed

Span 5: Outbox Relay → Kafka
  ├── duration: 3ms (async, separate trace linked by event_id)
  ├── operation: publish_event
  ├── topic: paygate.payments
  └── event: payment.captured

Span 6: Webhook Service → Merchant endpoint
  ├── duration: 340ms (async, separate trace)
  ├── operation: deliver_webhook
  ├── endpoint: https://merchant.com/webhooks
  ├── response_status: 200
  └── attempt: 1
```

Total synchronous latency: ~27ms (Spans 1-4)
Webhook delivery: ~500ms end-to-end (async)

---

## 10. Data at rest — what lives where

| Data | Storage | Encryption | Retention |
|------|---------|------------|-----------|
| Orders, payments, refunds | PostgreSQL | TDE (AES-256) | 7 years |
| Ledger entries | PostgreSQL (dedicated schema) | TDE (AES-256) | 7 years |
| Settlements | PostgreSQL | TDE (AES-256) | 7 years |
| Webhook delivery attempts | PostgreSQL | TDE | 90 days |
| Audit events | PostgreSQL → S3 archive | TDE + S3 SSE | 5 years |
| Card tokens (simulated) | Vault / encrypted column | AES-256-GCM, key in KMS | Until customer deletes |
| Idempotency keys | Redis | In-memory | 24 hours (TTL) |
| Rate limit counters | Redis | In-memory | 1-60 seconds (TTL) |
| Events in transit | Kafka | Broker-level TDE | 7-30 days (topic config) |
| Reports, exports | S3 / MinIO | SSE-S3 | 3 years |
| Request/response logs | S3 / Loki | SSE-S3 | 13 months |

---

## 11. Advanced distributed flows (optional track)

### 11.1 Saga-based capture (extracted services)

```
PAYMENT API  ->  SAGA ORCHESTRATOR  ->  LEDGER SERVICE  ->  PAYMENT SERVICE
    │                 │                      │                    │
    │ capture req     │ create saga          │                    │
    ├────────────────►│                      │                    │
    │                 │ emit command         │                    │
    │                 ├─────────────────────►│ post capture       │
    │                 │                      │ entries idempotent │
    │                 │◄─────────────────────┤ posted/rejected    │
    │                 │ emit result event    │                    │
    │                 ├───────────────────────────────────────────►│
    │                 │                      │ finalize payment    │
    │◄────────────────┤                      │ state + outbox      │
```

### 11.2 Schema rollout flow

```
PRODUCER CHANGE -> CI SCHEMA CHECK -> CONSUMER CONTRACT TESTS -> DUAL PUBLISH -> CUTOVER
```

### 11.3 Hold/release/commit flow

```
RISK ENGINE -> create hold -> payment delayed
OPS DECISION -> release hold OR commit hold
COMMIT -> ledger posting + settlement eligibility update
```
