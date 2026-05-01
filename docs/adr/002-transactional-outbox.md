# ADR-002: Transactional Outbox for Event Publishing

**Status:** Accepted
**Date:** 2026-05-01

## Context

PayGate must publish domain events (e.g., `payment.captured`, `order.expired`) to Kafka for downstream consumers (webhooks, analytics, reconciliation). Publishing directly to Kafka inside a business transaction is unsafe: if Kafka is unavailable, the transaction fails; if the DB commits but Kafka publish fails, the event is lost.

## Decision

Use the **transactional outbox pattern**:

1. Within the same Postgres transaction that changes domain state, write an event row to `public.outbox`
2. A background relay (`outbox.Relay`) polls unpublished rows using `SELECT ... FOR UPDATE SKIP LOCKED` and publishes them to Kafka
3. Once published, the relay sets `published_at = NOW()`

The outbox table schema:
```sql
CREATE TABLE public.outbox (
    id             BIGSERIAL PRIMARY KEY,
    aggregate_type TEXT      NOT NULL,
    aggregate_id   TEXT      NOT NULL,
    event_type     TEXT      NOT NULL,
    merchant_id    TEXT      NOT NULL,
    payload        JSONB     NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at   TIMESTAMPTZ
);
```

## Consequences

**Positive:**
- Atomicity: either the domain state change AND the event record commit together, or neither does
- At-least-once delivery: the relay retries unpublished rows on restart
- Kafka outages don't fail business transactions; rows accumulate and drain when Kafka recovers

**Negative:**
- Events are delivered with eventual consistency (relay poll interval latency, typically < 1s)
- Relay must handle duplicate publishes idempotently (downstream consumers must be idempotent)
- The outbox table grows unboundedly without a cleanup job (future work: archive rows older than 30 days)

## Alternatives Considered

- **Publish inside transaction:** Rejected — Kafka publish in a DB transaction creates a distributed transaction; Kafka unavailability fails payment writes
- **Debezium CDC:** Valid long-term option; rejected for now due to operational complexity of running a CDC connector
