# ADR-004: Idempotency Posture for Mutating API Calls

**Status:** Accepted
**Date:** 2026-05-01

## Context

Payment APIs are called over unreliable networks. Clients retry on timeout or network error, which can cause duplicate charges if the server processed the first request but the response was lost. We must handle the three cases:

1. **Novel request** — process and store result
2. **Duplicate before processing** — return same result without reprocessing
3. **Duplicate during processing** — wait or return conflict

## Decision

Require an `Idempotency-Key` header on all mutating payment endpoints (`POST /v1/orders`, `POST /v1/payments/authorize`, `POST /v1/payments/{id}/capture`).

The idempotency store uses a **two-tier architecture**:
- **Tier 1 (Redis, fast path):** `SET NX PX 30000` — prevents concurrent duplicates within 30 s window
- **Tier 2 (Postgres, durable):** `INSERT ... ON CONFLICT DO NOTHING` — stores `(merchant_id, key, status_code, response_body)` for replay after Redis expiry

On replay, the middleware:
1. Sets response status and body to the stored values
2. Adds `Idempotent-Replayed: true` header

If Redis is unavailable, the store falls back to Postgres-only for durability.

## Consequences

**Positive:**
- Clients can safely retry any mutation without risk of double-charge
- Fast in-memory check (< 1ms) on the hot path
- Durable Postgres fallback survives process restarts and Redis outages

**Negative:**
- Idempotency keys must be unique per merchant+operation, not globally; clients must generate good keys (UUID or KSUID recommended)
- Responses are replayed verbatim — a 500 on first try is also replayed, so transient errors "stick" for 30 s

## Alternatives Considered

- **Database-only idempotency:** Simpler but slower (one extra DB round-trip per request on hot path)
- **No idempotency enforcement:** Rejected — unacceptable for a payment platform where duplicates cause real financial harm
