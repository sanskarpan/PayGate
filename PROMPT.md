# PayGate — Claude Code Prompt

> Use this as a system prompt or project context for Claude Code when building PayGate.

---

## Project prompt

```markdown
You are building PayGate, a production-grade multi-tenant payment platform inspired by Razorpay. This is a portfolio project targeting SDE2/SDE3 level. It is NOT a checkout demo — it is a payments backbone with state machines, double-entry ledger, webhook delivery, settlements, and reconciliation.

## Tech stack
- **Backend**: Go 1.22+ (all services)
- **Frontend**: Next.js 14+ with App Router, TypeScript, Tailwind CSS, shadcn/ui
- **Database**: PostgreSQL 16 (single cluster, per-service schemas)
- **Event bus**: Apache Kafka (KRaft mode, no ZooKeeper)
- **Cache**: Redis 7
- **Object storage**: MinIO (S3-compatible)
- **Containerization**: Docker + Docker Compose for local dev
- **Observability**: OpenTelemetry, Prometheus, Grafana
- **Testing**: Go testing + testcontainers-go, k6 for load tests

## Architecture
Event-driven microservices with shared Postgres. Services communicate via gRPC (sync) and Kafka (async). Every state change writes to a transactional outbox table; a relay worker publishes to Kafka. The frontend talks to services through an API gateway.

Services: api-gateway, order-service, payment-service, refund-service, settlement-service, ledger-service, webhook-service, recon-worker, outbox-relay, gateway-proxy (simulator)

## Critical design rules — follow these on every file you write:

### 1. State machines are explicit
Every entity with a lifecycle (order, payment, refund, settlement, webhook delivery) has a state machine defined as a Go type with a Transition function. No boolean flags as state. No implicit transitions. Invalid transitions return errors.

```go
type PaymentState string
const (
    PaymentCreated      PaymentState = "created"
    PaymentAuthorized   PaymentState = "authorized"
    PaymentCaptured     PaymentState = "captured"
    PaymentFailed       PaymentState = "failed"
    PaymentAutoRefunded PaymentState = "auto_refunded"
)

func (s PaymentState) Transition(event PaymentEvent) (PaymentState, error) {
    // explicit transition table, returns error on invalid transition
}
```

### 2. Double-entry ledger
Every monetary operation (capture, refund, settlement) creates ledger entries where total debits = total credits. Ledger entries are append-only — no UPDATE, no DELETE. Corrections are compensating entries. The Ledger Service enforces this via a CHECK constraint and application-level validation.

Journal entries for a capture of ₹500 at 2% fee:
```
Dr. CUSTOMER_RECEIVABLE     ₹500
  Cr. MERCHANT_PAYABLE        ₹490
  Cr. PLATFORM_FEE_REVENUE    ₹10
```

### 3. Transactional outbox
NEVER publish to Kafka directly from a service handler. Always:
1. In the same Postgres transaction as the state change, INSERT into the outbox table
2. A separate outbox-relay process polls the outbox and publishes to Kafka
3. Mark `published_at` on success
4. Consumers must be idempotent (deduplicate by event_id)

### 4. Idempotency
All POST endpoints accept `Idempotency-Key` header. Implementation:
- Redis key: `idempotency:{merchant_id}:{endpoint}:{client_key}`
- SET NX EX 86400 (24h TTL)
- New key → execute, cache response
- Existing + completed → return cached response + Idempotent-Replayed header
- Existing + in_progress → 409 Conflict + Retry-After: 1

### 5. ID format
All public IDs use KSUID with entity prefix: order_xxx, pay_xxx, rfnd_xxx, sttl_xxx, evt_xxx, merch_xxx

### 6. Error format
```json
{
  "error": {
    "code": "BAD_REQUEST_ERROR",
    "description": "human readable message",
    "field": "amount",
    "source": "business",
    "step": "payment_initiation",
    "reason": "input_validation_failed"
  }
}
```

### 7. Webhook signatures
HMAC-SHA256 of the raw request body using the subscription's webhook secret. Header: X-PayGate-Signature.

### 8. Never store sensitive data in logs
All logging passes through a scrubber that removes: card numbers, CVV, key_secret, webhook secrets, passwords. Scrub BEFORE writing, not after.

### 9. Testing requirements
- Every state machine: table-driven tests for all valid + invalid transitions
- Every ledger operation: assert debit == credit
- Every API endpoint: contract test for response shape
- Integration tests use testcontainers-go for real Postgres/Redis
- No mocks for the ledger — always test against real DB

### 10. Code organization per service
```
internal/{service}/
├── domain.go          # types, state machine, business rules (no deps)
├── service.go         # use cases, orchestration
├── repository.go      # database queries (interface)
├── postgres.go        # Postgres implementation of repository
├── handler.go         # HTTP/gRPC handlers
├── handler_test.go    # handler tests
├── domain_test.go     # domain logic tests
└── service_test.go    # service tests with mocked repo
```

## Phase plan
Build in this order. Do not skip phases.

**Phase 1**: Merchant + API keys → Order service → Payment service (authorize + capture) → Ledger (basic) → Gateway proxy (simulator) → Checkout page → Dashboard (basic)

**Phase 2**: Idempotency middleware → Outbox relay → Refund service → Webhook service (delivery + retry + replay) → Settlement service (batch + ledger) → Reconciliation worker → Dashboard (refunds, webhooks, settlements)

**Phase 3**: RBAC → Audit logging → Risk engine (velocity + rules) → Request scrubbing → Rate limiting → Dashboard (risk, audit, team)

**Phase 4**: Disputes → Advanced settlements → Chaos tests → Load tests → Observability dashboards → Documentation

## Database conventions
- All timestamps: TIMESTAMPTZ (not TIMESTAMP)
- All monetary amounts: BIGINT in smallest currency unit (paise for INR)
- All IDs: TEXT with KSUID prefix
- All status fields: TEXT with CHECK constraint (not enum type — easier to migrate)
- Every table has: created_at, updated_at
- Ledger tables are append-only: no UPDATE/DELETE granted to service role

## API conventions
- REST with JSON (external APIs)
- gRPC (internal service-to-service)
- Cursor-based pagination (not offset): use `created_at` cursor
- Standard headers: Authorization (Basic), Idempotency-Key, X-Request-Id
- Response wrapper: `{ "entity": "collection", "count": N, "items": [...], "has_more": bool }`

## When I ask you to build a specific service:
1. Start with the domain.go file (types, state machine, validation)
2. Write domain_test.go immediately (table-driven state machine tests)
3. Then repository interface and Postgres implementation
4. Then service.go with business logic
5. Then handler.go with HTTP/gRPC endpoints
6. Then integration test
7. Then the database migration file

Always write tests alongside the code, not after. The state machine test is the FIRST test you write for any new entity.
```

---

## Usage instructions

### Option A: Project-level context

Save the prompt above as `CLAUDE.md` in the project root. Claude Code will read it automatically.

### Option B: Per-session context

When starting a Claude Code session, paste the prompt as the first message, then follow up with specific tasks like:

- "Build the order service — domain, repository, handler, and tests"
- "Implement the outbox relay worker"
- "Create the webhook delivery engine with retry logic"
- "Write the settlement batch job"
- "Add idempotency middleware"

### Recommended task sequence (Phase 1)

```
Task 1: "Set up the project structure, docker-compose.yml, Makefile, and initial database migrations for the merchants schema"

Task 2: "Build the merchant service — registration, API key generation, auth middleware"

Task 3: "Build the order service — domain model with state machine, repository, handler, tests, and migration"

Task 4: "Build the simulated gateway proxy — configurable success/failure/timeout/delay behaviors"

Task 5: "Build the payment service — domain model, state machine, authorization flow, capture flow, ledger integration, tests"

Task 6: "Build the basic ledger service — account setup, entry creation with double-entry validation, balance queries, gRPC API"

Task 7: "Create the checkout page — simple HTML/JS that creates a payment against an order"

Task 8: "Build the API gateway — routing, auth, rate limiting, correlation IDs, logging"

Task 9: "Build the basic merchant dashboard — login, orders list, payment detail pages"

Task 10: "Write integration tests for the full Phase 1 flow: order → payment → capture → ledger entries"
```

### Recommended task sequence (Phase 2)

```
Task 11: "Add idempotency middleware with Redis backend — all three cases (new, completed, in-progress)"

Task 12: "Build the outbox relay worker — poll outbox table, publish to Kafka, mark published"

Task 13: "Build the refund service — domain, state machine, eligibility validation, concurrent refund protection, ledger entries"

Task 14: "Build the webhook service — subscription management, Kafka consumer, HTTP delivery, signature generation"

Task 15: "Add webhook retry engine — exponential backoff, Redis sorted set, retry worker, dead-letter queue"

Task 16: "Build the settlement service — nightly batch, fee calculation, settlement items, ledger entries"

Task 17: "Build the reconciliation worker — three-way match, mismatch detection, batch recording"

Task 18: "Add webhook replay endpoint and dashboard webhook delivery log"

Task 19: "Build dashboard pages for refunds, webhooks, settlements, and reconciliation"

Task 20: "Write Phase 2 integration tests — refund flow, webhook delivery, settlement batch, reconciliation"
```

---

## Troubleshooting prompts

If Claude Code produces code that violates the design rules, use these correction prompts:

**If it uses boolean flags instead of state machines:**
> "The payment status should be an explicit state machine with a Transition function, not boolean flags. Refactor to use the state machine pattern from the design rules."

**If it publishes to Kafka directly:**
> "Never publish to Kafka directly from a handler. Use the transactional outbox pattern: write to the outbox table in the same Postgres transaction, then the relay worker publishes."

**If ledger entries don't balance:**
> "Every ledger transaction must have total debits equal to total credits. Add a validation check in the ledger service and a unit test that asserts balance."

**If it skips tests:**
> "Write the domain_test.go with table-driven state machine tests before writing the handler. Tests are not optional for payment systems."

**If it uses UUIDs instead of KSUIDs:**
> "Use KSUID with entity prefix (order_xxx, pay_xxx) instead of UUID v4. KSUIDs are time-sortable which is important for cursor pagination."

**If it stores card data outside the CDE:**
> "Card numbers must never leave the tokenization vault. The payment service only sees tokens (tok_xxxx1234). Scrub any raw card data from logs."
