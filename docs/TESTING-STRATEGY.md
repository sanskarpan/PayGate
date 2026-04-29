# PayGate — Testing Strategy

> How to test a payment system where bugs cost real money.

---

## 1. Testing pyramid

```
                    ▲
                   / \
                  / E2E\          5-10 tests
                 /───────\        (slow, fragile, high confidence)
                / Integra-\       30-50 tests
               /   tion    \      (medium speed, real deps)
              /─────────────\
             /    Contract   \    20-30 tests
            /─────────────────\   (API shape verification)
           /       Unit        \  200+ tests
          /─────────────────────\ (fast, isolated, high coverage)
```

---

## 2. Unit tests

### 2.1 What to unit test

**State machines** — the most critical unit tests in the entire project:

```go
// payment_state_test.go
func TestPaymentStateMachine(t *testing.T) {
    tests := []struct {
        name        string
        fromState   PaymentState
        event       PaymentEvent
        expectState PaymentState
        expectError bool
    }{
        {"authorize from created", StateCreated, EventAuthSuccess, StateAuthorized, false},
        {"capture from authorized", StateAuthorized, EventCaptureSuccess, StateCaptured, false},
        {"capture from created is invalid", StateCreated, EventCaptureSuccess, "", true},
        {"double capture is invalid", StateCaptured, EventCaptureSuccess, "", true},
        {"refund from authorized is invalid", StateAuthorized, EventRefundRequested, "", true},
        {"auto-refund from authorized", StateAuthorized, EventCaptureExpired, StateAutoRefunded, false},
        {"fail from created", StateCreated, EventAuthFailed, StateFailed, false},
        {"no transition from terminal states", StateFailed, EventAuthSuccess, "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            next, err := Transition(tt.fromState, tt.event)
            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expectState, next)
            }
        })
    }
}
```

**Ledger entry creation** — validate double-entry invariants:

```go
func TestLedgerEntry_DebitEqualsCreditPerTransaction(t *testing.T) {
    entries := CreateCaptureEntries(payment)
    totalDebit := sumDebits(entries)
    totalCredit := sumCredits(entries)
    assert.Equal(t, totalDebit, totalCredit, "debit must equal credit")
}

func TestLedgerEntry_RefundReversesCapture(t *testing.T) {
    captureEntries := CreateCaptureEntries(payment)
    refundEntries := CreateFullRefundEntries(payment, refund)

    // After both sets of entries, merchant payable should be zero
    balance := computeAccountBalance("MERCHANT_PAYABLE", append(captureEntries, refundEntries...))
    assert.Equal(t, int64(0), balance)
}
```

**Fee calculation**:

```go
func TestFeeCalculation(t *testing.T) {
    tests := []struct{
        amount     int64
        feeRate    float64
        expectFee  int64
    }{
        {50000, 0.02, 1000},       // 2% of ₹500
        {100, 0.02, 2},            // 2% of ₹1
        {99, 0.02, 2},             // rounds up
        {1, 0.02, 1},              // minimum fee = 1 paisa
    }
    for _, tt := range tests {
        fee := CalculatePlatformFee(tt.amount, tt.feeRate)
        assert.Equal(t, tt.expectFee, fee)
    }
}
```

**Webhook signature generation and verification**:

```go
func TestWebhookSignature_RoundTrip(t *testing.T) {
    secret := "test_webhook_secret"
    body := []byte(`{"event":"payment.captured","payload":{}}`)

    sig := GenerateSignature(secret, body)
    assert.True(t, VerifySignature(secret, body, sig))
}

func TestWebhookSignature_RejectsModifiedBody(t *testing.T) {
    secret := "test_webhook_secret"
    body := []byte(`{"event":"payment.captured"}`)
    sig := GenerateSignature(secret, body)

    modified := []byte(`{"event":"payment.captured","extra":"field"}`)
    assert.False(t, VerifySignature(secret, modified, sig))
}
```

**Refund eligibility validation**:

```go
func TestRefundEligibility(t *testing.T) {
    tests := []struct{
        name           string
        paymentStatus  string
        amountCaptured int64
        amountRefunded int64
        refundAmount   int64
        expectError    string
    }{
        {"full refund on captured", "captured", 50000, 0, 50000, ""},
        {"partial refund", "captured", 50000, 0, 25000, ""},
        {"exceeds remaining", "captured", 50000, 30000, 25000, "exceeds refundable amount"},
        {"not captured", "authorized", 50000, 0, 50000, "payment not captured"},
        {"already fully refunded", "captured", 50000, 50000, 1, "already fully refunded"},
        {"zero amount", "captured", 50000, 0, 0, "amount must be positive"},
    }
    // ...
}
```

### 2.2 Unit test conventions

- Run with `go test ./...` — target < 2 seconds for the full suite
- No database, no Redis, no Kafka — use interfaces and mocks
- Table-driven tests for state machines and validation
- Every public function has at least one test
- Coverage target: 80% for service packages, 95% for state machine and ledger packages

---

## 3. Integration tests

### 3.1 Infrastructure

Use `testcontainers-go` to spin up real PostgreSQL, Redis, and Kafka instances in Docker:

```go
func TestIntegration_PaymentCaptureCreatesLedgerEntries(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    ctx := context.Background()
    pg := startPostgres(t)       // testcontainer
    redis := startRedis(t)       // testcontainer

    // Run migrations
    runMigrations(t, pg.DSN)

    // Create service instances with real connections
    ledgerSvc := ledger.New(pg.Pool)
    paymentSvc := payment.New(pg.Pool, redis.Client, ledgerSvc, mockGateway)

    // Execute flow
    order := createTestOrder(t, pg.Pool, testMerchant)
    pay := authorizeTestPayment(t, paymentSvc, order)
    captured, err := paymentSvc.Capture(ctx, pay.ID, pay.Amount)

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "captured", captured.Status)

    // Check ledger entries were created
    entries, _ := ledgerSvc.GetEntriesBySource(ctx, "payment", pay.ID)
    assert.Len(t, entries, 3) // debit + 2 credits
    assertDebitEqualsCredit(t, entries)
}
```

### 3.2 What to integration test

| Test | Services involved | Asserts |
|------|------------------|---------|
| Order → Payment → Capture | Order, Payment, Ledger | Ledger entries exist and balance |
| Capture → Refund | Payment, Refund, Ledger | Compensating entries created, payment.amount_refunded updated |
| Capture → Settlement | Payment, Settlement, Ledger | Settlement items match, payments marked settled |
| Outbox → Kafka publish | Outbox relay, Kafka | Event appears in topic within 5s |
| Webhook delivery | Webhook service, mock endpoint | HTTP POST received with correct signature |
| Idempotency | Any write endpoint, Redis | Duplicate request returns same response |
| Rate limiting | API gateway, Redis | 429 returned after burst |
| Reconciliation | Recon worker, all schemas | Mismatches detected for intentionally broken data |

### 3.3 Integration test patterns

- Each test gets a fresh database schema (use `CREATE SCHEMA test_{uuid}`)
- Tests are parallelizable by using separate merchant IDs
- Tests run with `go test -tags=integration ./...`
- CI runs integration tests in a separate stage with Docker Compose

---

## 4. Contract tests

### 4.1 API contract tests

Verify that API responses match the documented schema:

```go
func TestAPIContract_CreateOrder(t *testing.T) {
    resp := httptest.NewRecorder()
    req := createOrderRequest(t, validOrderPayload)

    router.ServeHTTP(resp, req)

    assert.Equal(t, 201, resp.Code)

    var body map[string]interface{}
    json.Unmarshal(resp.Body.Bytes(), &body)

    // Required fields exist
    assert.Contains(t, body, "id")
    assert.Contains(t, body, "entity")
    assert.Contains(t, body, "amount")
    assert.Contains(t, body, "status")
    assert.Contains(t, body, "created_at")

    // Types are correct
    assert.Equal(t, "order", body["entity"])
    assert.Equal(t, "created", body["status"])
    assert.IsType(t, float64(0), body["amount"])

    // ID format
    assert.True(t, strings.HasPrefix(body["id"].(string), "order_"))
}
```

### 4.2 Webhook payload contract tests

Verify webhook payloads match the documented schema for each event type:

```go
func TestWebhookContract_PaymentCaptured(t *testing.T) {
    event := buildWebhookEvent("payment.captured", testPayment)

    // Validate against JSON schema
    assert.NoError(t, validateAgainstSchema(event, "schemas/payment.captured.json"))

    // Required fields
    assert.Equal(t, "event", event.Entity)
    assert.NotEmpty(t, event.EventID)
    assert.Equal(t, "payment.captured", event.Event)
    assert.NotEmpty(t, event.Payload.Payment.Entity.ID)
    assert.Equal(t, "captured", event.Payload.Payment.Entity.Status)
}
```

### 4.3 gRPC contract tests

For internal service-to-service calls, use protobuf schema validation:

```go
func TestGRPCContract_LedgerCreateEntries(t *testing.T) {
    req := &ledgerpb.CreateEntriesRequest{
        TransactionId: "txn_test",
        MerchantId:    "merch_test",
        Entries: []*ledgerpb.Entry{
            {AccountCode: "CUSTOMER_RECEIVABLE", DebitAmount: 50000},
            {AccountCode: "MERCHANT_PAYABLE", CreditAmount: 49000},
            {AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: 1000},
        },
    }

    // Should not error — schema is valid
    _, err := proto.Marshal(req)
    assert.NoError(t, err)
}
```

---

## 5. End-to-end tests

### 5.1 Full flow tests

Run against a fully deployed local environment (Docker Compose):

```go
func TestE2E_HappyPath_OrderThroughSettlement(t *testing.T) {
    client := NewPayGateClient(baseURL, testAPIKey, testAPISecret)

    // 1. Create order
    order, err := client.CreateOrder(ctx, CreateOrderRequest{
        Amount:   50000,
        Currency: "INR",
        Receipt:  "e2e_test_001",
    })
    require.NoError(t, err)
    assert.Equal(t, "created", order.Status)

    // 2. Simulate payment (via test checkout endpoint)
    payment, err := client.SimulatePayment(ctx, order.ID, "card")
    require.NoError(t, err)
    assert.Equal(t, "authorized", payment.Status)

    // 3. Capture payment
    captured, err := client.CapturePayment(ctx, payment.ID, 50000)
    require.NoError(t, err)
    assert.Equal(t, "captured", captured.Status)

    // 4. Verify webhook was delivered
    // (webhook test endpoint records received events)
    eventually(t, 10*time.Second, func() bool {
        events := getReceivedWebhooks(t, order.ID)
        return containsEvent(events, "payment.captured")
    })

    // 5. Trigger settlement
    triggerSettlementBatch(t)

    // 6. Verify settlement created
    settlements, err := client.ListSettlements(ctx, merchant.ID)
    require.NoError(t, err)
    assert.NotEmpty(t, settlements)

    // 7. Run reconciliation
    triggerReconciliation(t)

    // 8. Verify no mismatches
    recon := getLatestReconBatch(t)
    assert.Equal(t, 0, recon.Mismatches)
}
```

### 5.2 Failure path tests

```go
func TestE2E_GatewayTimeout_PaymentFails(t *testing.T) {
    // Configure gateway simulator to timeout
    setGatewayMode(t, "timeout")

    order, _ := client.CreateOrder(ctx, orderReq)
    payment, err := client.SimulatePayment(ctx, order.ID, "card")

    assert.Error(t, err)
    // Verify order is still in "attempted" state, not "paid"
    order, _ = client.GetOrder(ctx, order.ID)
    assert.Equal(t, "attempted", order.Status)
}

func TestE2E_DuplicateCallback_NoDoubleCounting(t *testing.T) {
    setGatewayMode(t, "duplicate")

    order, _ := client.CreateOrder(ctx, orderReq)
    payment, _ := client.SimulatePayment(ctx, order.ID, "card")
    client.CapturePayment(ctx, payment.ID, 50000)

    // Even with duplicate callback, only one ledger entry set
    entries := getLedgerEntries(t, "payment", payment.ID)
    assert.Len(t, entries, 3) // exactly 3, not 6
}
```

---

## 6. Chaos tests

### 6.1 Tools

- **Toxiproxy**: inject network latency, timeouts, and connection resets between services
- **Database kill**: stop PostgreSQL mid-transaction to test outbox recovery
- **Kafka broker kill**: stop a Kafka broker to test producer retries

### 6.2 Chaos scenarios

| Scenario | Setup | Expected behavior |
|----------|-------|------------------|
| DB down during capture | Toxiproxy: close Postgres port | Capture returns 503, payment stays `authorized`, retryable |
| Kafka down during outbox relay | Stop Kafka broker | Outbox entries accumulate, relay resumes when Kafka returns |
| Redis down during idempotency check | Toxiproxy: close Redis port | Request proceeds (fail-open), logs warning |
| Webhook endpoint slow (30s) | Mock endpoint with 30s delay | Timeout after 10s, recorded as failed, retry scheduled |
| Network partition between services | Toxiproxy: add 5s latency | gRPC deadline exceeded, circuit breaker opens |
| Process crash mid-capture | Kill payment service after DB write but before response | Outbox entry exists, event will be published on recovery |

### 6.3 Chaos test runner

```go
func TestChaos_OutboxRecovery(t *testing.T) {
    // 1. Stop Kafka
    stopKafka(t)

    // 2. Capture a payment (writes to outbox but can't publish)
    payment := capturePayment(t, testPayment)
    assert.Equal(t, "captured", payment.Status)

    // 3. Verify outbox has unpublished entry
    count := countUnpublishedOutbox(t)
    assert.Equal(t, 1, count)

    // 4. Restart Kafka
    startKafka(t)

    // 5. Wait for relay to catch up
    eventually(t, 30*time.Second, func() bool {
        return countUnpublishedOutbox(t) == 0
    })

    // 6. Verify event reached Kafka
    events := consumeFromTopic(t, "paygate.payments")
    assert.Contains(t, eventTypes(events), "payment.captured")
}
```

---

## 7. Load tests

### 7.1 Tool

Use `k6` for load testing:

```javascript
// load_test_orders.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 100 },   // ramp up
        { duration: '5m', target: 100 },   // sustained
        { duration: '1m', target: 500 },   // spike
        { duration: '5m', target: 500 },   // sustained spike
        { duration: '2m', target: 0 },     // ramp down
    ],
    thresholds: {
        http_req_duration: ['p(99)<300'],   // p99 < 300ms
        http_req_failed: ['rate<0.01'],     // <1% errors
    },
};

export default function () {
    const payload = JSON.stringify({
        amount: Math.floor(Math.random() * 100000) + 100,
        currency: 'INR',
        receipt: `load_test_${__ITER}`,
    });

    const res = http.post(`${__ENV.BASE_URL}/v1/orders`, payload, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Basic ${__ENV.AUTH_TOKEN}`,
            'Idempotency-Key': `load_${__VU}_${__ITER}`,
        },
    });

    check(res, {
        'status is 201': (r) => r.status === 201,
        'has order id': (r) => JSON.parse(r.body).id.startsWith('order_'),
    });

    sleep(0.1);
}
```

### 7.2 Load test targets

| Endpoint | Target RPS | p99 latency | Error rate |
|----------|-----------|-------------|------------|
| POST /v1/orders | 1000 | < 50ms | < 0.1% |
| POST /v1/payments/{id}/capture | 500 | < 300ms | < 0.1% |
| POST /v1/payments/{id}/refunds | 200 | < 100ms | < 0.1% |
| GET /v1/payments | 2000 | < 30ms | < 0.01% |

---

## 8. CI pipeline

```yaml
stages:
  - lint:        # golangci-lint, eslint
  - unit:        # go test ./... -short (< 2 min)
  - build:       # go build, docker build
  - integration: # go test -tags=integration (Docker Compose, < 10 min)
  - contract:    # API + webhook schema tests (< 2 min)
  - e2e:         # full flow tests against Docker Compose env (< 15 min)
  - load:        # k6 smoke test (1 min, low concurrency, verify no regressions)
```

Chaos tests run weekly in a dedicated staging environment, not in CI.
