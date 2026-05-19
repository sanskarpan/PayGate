# Integration Guide

## 1. Create merchant credentials

1. Create a merchant and an API key.
2. Use HTTP Basic Auth with `key_id:key_secret`.
3. Prefer a dedicated `write` key for server-side order/payment creation and a `read` or `admin` key for operator tooling.

## 2. Standard payment flow

1. `POST /v1/orders`
2. `POST /v1/payments`
3. `POST /v1/payments/{paymentID}/capture`
4. `GET /v1/payments/{paymentID}`

The synchronous money path is durable inside Postgres: order state, payment state, ledger entries, and outbox event are committed together.

## 3. Webhooks

1. `POST /v1/webhooks` with an `https` URL.
2. Verify the signature header using the subscription secret.
3. Treat delivery as at-least-once. Deduplicate on `event_id`.
4. Parse the event according to [`docs/WEBHOOK-EVENT-CATALOG.md`](/Users/sanskar/dev/PayGate/docs/WEBHOOK-EVENT-CATALOG.md:1).

## 4. Refunds

1. `POST /v1/refunds`
2. Poll `GET /v1/refunds` or consume the `refund.processed` webhook event.

## 5. Settlements and payouts

1. Settlements are created by the settlement worker or batch path.
2. Initiate payout with `POST /v1/settlements/{settlementID}/payout`.
3. Inspect payout lifecycle with `GET /v1/payouts/{payoutID}/events`.

## 6. Idempotency

- Send `Idempotency-Key` on every money-changing write.
- Duplicate requests return the durable previous result instead of re-posting money movement.

## 7. Error handling

- Retry `5xx`, network timeouts, and `429`.
- Do not blindly retry `4xx` business errors.
- Use webhook replay or saga replay for async/operator recovery instead of direct database edits.
