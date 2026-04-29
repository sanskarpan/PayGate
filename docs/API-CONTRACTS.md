# PayGate — API Contracts

> Complete API reference for all external-facing endpoints. Every request/response shape, header, and status code.

---

## 1. Common conventions

### Authentication
All endpoints require HTTP Basic Auth unless marked as public.
```
Authorization: Basic base64({key_id}:{key_secret})
```

### Standard headers

| Header | Direction | Required | Description |
|--------|-----------|----------|-------------|
| `Authorization` | Request | Yes | Basic auth with API key |
| `Idempotency-Key` | Request | POST only | Client-generated unique key (max 64 chars) |
| `X-Request-Id` | Response | Always | Correlation ID (injected by gateway) |
| `Idempotent-Replayed` | Response | Conditional | `true` when returning cached idempotent response |
| `X-RateLimit-Limit` | Response | Always | Requests allowed per window |
| `X-RateLimit-Remaining` | Response | Always | Requests remaining in window |
| `Retry-After` | Response | On 429/409 | Seconds to wait before retry |

### Pagination

List endpoints use cursor-based pagination:

```
GET /v1/orders?count=10&from={unix_timestamp}&to={unix_timestamp}

Response:
{
  "entity": "collection",
  "count": 10,
  "items": [...],
  "has_more": true
}
```

- `count`: items per page (default 10, max 100)
- `from`: created_at >= this timestamp (inclusive)
- `to`: created_at <= this timestamp (inclusive)

### Error responses

All errors follow this shape:
```json
{
  "error": {
    "code": "BAD_REQUEST_ERROR",
    "description": "The amount field is required and must be a positive integer",
    "field": "amount",
    "source": "business",
    "step": "order_creation",
    "reason": "input_validation_failed",
    "metadata": {}
  }
}
```

| Code | HTTP Status | When |
|------|-------------|------|
| `BAD_REQUEST_ERROR` | 400 | Invalid input, missing fields |
| `UNAUTHORIZED` | 401 | Invalid or missing API key |
| `FORBIDDEN` | 403 | Valid key but insufficient scope |
| `NOT_FOUND` | 404 | Resource doesn't exist or belongs to another merchant |
| `IDEMPOTENCY_CONFLICT` | 409 | Idempotency key in use, request in progress |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `GATEWAY_ERROR` | 502 | Payment gateway returned an error |
| `SERVER_ERROR` | 500 | Internal error |

---

## 2. Orders API

### Create order
```
POST /v1/orders
```

**Request:**
```json
{
  "amount": 50000,
  "currency": "INR",
  "receipt": "rcpt_2024_001",
  "notes": {
    "policy_id": "POL-123"
  },
  "partial_payment": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `amount` | integer | Yes | Amount in smallest currency unit (paise). Min: 100 |
| `currency` | string | Yes | ISO 4217 code. Supported: `INR`, `USD` |
| `receipt` | string | No | Merchant reference (max 40 chars) |
| `notes` | object | No | Key-value pairs (max 15 keys, 256 chars per value) |
| `partial_payment` | boolean | No | Allow partial payments (default: false) |

**Response: 201 Created**
```json
{
  "id": "order_LxR4k9mNp2vQ",
  "entity": "order",
  "amount": 50000,
  "amount_paid": 0,
  "amount_due": 50000,
  "currency": "INR",
  "receipt": "rcpt_2024_001",
  "status": "created",
  "partial_payment": false,
  "notes": { "policy_id": "POL-123" },
  "created_at": 1714000000
}
```

### Fetch order
```
GET /v1/orders/{order_id}
```

**Response: 200 OK** — same shape as create response, with updated `status`, `amount_paid`, `amount_due`.

### List orders
```
GET /v1/orders?count=10&from=1714000000&to=1714100000
```

**Response: 200 OK**
```json
{
  "entity": "collection",
  "count": 10,
  "items": [ /* order objects */ ],
  "has_more": true
}
```

---

## 3. Payments API

### Fetch payment
```
GET /v1/payments/{payment_id}
```

**Response: 200 OK**
```json
{
  "id": "pay_Mn3qR7sWx1yZ",
  "entity": "payment",
  "amount": 50000,
  "currency": "INR",
  "status": "captured",
  "order_id": "order_LxR4k9mNp2vQ",
  "method": "card",
  "captured": true,
  "refund_status": "none",
  "amount_refunded": 0,
  "fee": 1000,
  "tax": 0,
  "card_token": "tok_xxxx1234",
  "notes": {},
  "error_code": null,
  "error_description": null,
  "authorized_at": 1714000050,
  "captured_at": 1714000100,
  "created_at": 1714000050
}
```

### Capture payment
```
POST /v1/payments/{payment_id}/capture
```

**Precondition:** Payment must be in `authorized` state.

**Request:**
```json
{
  "amount": 50000,
  "currency": "INR"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `amount` | integer | Yes | Must match authorized amount (or less if partial capture enabled) |
| `currency` | string | Yes | Must match original currency |

**Response: 200 OK** — full payment object with `status: "captured"`

**Error cases:**
- Payment not in `authorized` state → `400 BAD_REQUEST_ERROR`
- Amount exceeds authorized amount → `400 BAD_REQUEST_ERROR`
- Currency mismatch → `400 BAD_REQUEST_ERROR`

### List payments
```
GET /v1/payments?count=10&from=1714000000&to=1714100000
```

---

## 4. Refunds API

### Create refund
```
POST /v1/payments/{payment_id}/refunds
```

**Precondition:** Payment must be in `captured` state with remaining refundable amount.

**Request:**
```json
{
  "amount": 25000,
  "speed": "normal",
  "receipt": "rfnd_rcpt_001",
  "notes": {
    "reason": "customer_request"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `amount` | integer | Yes | In paise. Must be ≤ (captured - already_refunded) |
| `speed` | string | No | `normal` (default) or `optimum` |
| `receipt` | string | No | Merchant reference |
| `notes` | object | No | Key-value pairs |

**Response: 201 Created**
```json
{
  "id": "rfnd_Kp5tV2wXz8aB",
  "entity": "refund",
  "payment_id": "pay_Mn3qR7sWx1yZ",
  "amount": 25000,
  "currency": "INR",
  "status": "created",
  "speed_requested": "normal",
  "speed_processed": null,
  "receipt": "rfnd_rcpt_001",
  "notes": { "reason": "customer_request" },
  "created_at": 1714001000
}
```

### Fetch refund
```
GET /v1/refunds/{refund_id}
```

### List refunds for a payment
```
GET /v1/payments/{payment_id}/refunds
```

---

## 5. Settlements API

### List settlements
```
GET /v1/settlements?count=10&from=1714000000&to=1714100000
```

**Response: 200 OK**
```json
{
  "entity": "collection",
  "count": 1,
  "items": [
    {
      "id": "sttl_Jq8uY3xAb5cD",
      "entity": "settlement",
      "amount_gross": 500000,
      "amount_fees": 10000,
      "amount_tax": 1800,
      "amount_refunds": 25000,
      "amount_net": 463200,
      "currency": "INR",
      "status": "processed",
      "payment_count": 10,
      "cycle_start": 1713900000,
      "cycle_end": 1713986400,
      "utr": "UTR123456789",
      "processed_at": 1714003600,
      "created_at": 1714002000
    }
  ],
  "has_more": false
}
```

### Fetch settlement with items
```
GET /v1/settlements/{settlement_id}
```

Returns settlement object plus `items` array with per-payment breakdown.

---

## 6. Webhooks API

### Create subscription
```
POST /v1/webhooks
```

**Request:**
```json
{
  "url": "https://merchant.com/webhooks/paygate",
  "events": ["payment.captured", "payment.failed", "refund.processed"],
  "secret": "whsec_merchant_provided_secret"
}
```

**Response: 201 Created**
```json
{
  "id": "wsub_xxx",
  "entity": "webhook_subscription",
  "url": "https://merchant.com/webhooks/paygate",
  "events": ["payment.captured", "payment.failed", "refund.processed"],
  "status": "active",
  "created_at": 1714000000
}
```

### List subscriptions
```
GET /v1/webhooks
```

### Replay event
```
POST /v1/webhooks/events/{event_id}/replay
```

Re-delivers the event to all matching subscriptions. Bypasses duplicate suppression. Returns `202 Accepted`.

### Webhook delivery payload (what merchant receives)

```
POST https://merchant.com/webhooks/paygate
Content-Type: application/json
X-PayGate-Signature: {hmac_sha256_hex}
X-PayGate-Event-Id: evt_xxx

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
        "method": "card",
        "captured_at": 1714000100
      }
    }
  },
  "created_at": 1714000100
}
```

**Signature verification (merchant-side pseudocode):**
```
expected = HMAC-SHA256(webhook_secret, raw_request_body)
actual = request.headers["X-PayGate-Signature"]
if constant_time_compare(expected, actual): accept
else: reject with 401
```

---

## 7. Merchant management API

### Create merchant (admin only)
```
POST /v1/merchants
```

### Generate API key
```
POST /v1/merchants/{merchant_id}/keys
```

**Response: 201 Created**
```json
{
  "key_id": "rzp_test_Fs5xB6aDe8fG",
  "key_secret": "secret_only_shown_once_abc123xyz",
  "mode": "test",
  "scope": "write",
  "created_at": 1714000000
}
```

**Important:** `key_secret` is returned only once. Store it immediately. It is stored as a bcrypt hash in the database and cannot be retrieved again.

### Revoke API key
```
DELETE /v1/merchants/{merchant_id}/keys/{key_id}
```

Returns `200 OK`. Key immediately becomes invalid.

---

## 8. Webhook event catalog

| Event | Trigger | Payload contains |
|-------|---------|-----------------|
| `order.created` | Order created | order |
| `order.paid` | Payment captured for order | order, payment |
| `payment.authorized` | Gateway auth success | payment |
| `payment.captured` | Capture succeeds | payment |
| `payment.failed` | Auth or capture fails | payment |
| `refund.created` | Refund initiated | refund, payment |
| `refund.processed` | Refund confirmed by gateway | refund, payment |
| `refund.failed` | Refund rejected | refund, payment |
| `settlement.created` | Settlement batch created | settlement |
| `settlement.processed` | Settlement confirmed | settlement |
| `dispute.created` | Chargeback received | dispute, payment |
| `dispute.won` | Dispute resolved in merchant's favor | dispute |
| `dispute.lost` | Dispute resolved against merchant | dispute |
