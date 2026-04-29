# PayGate — Foundation Review

> Direct assessment of the current documentation foundation: what is solid, what was assumption-heavy, and what must be proven during implementation.

---

## Verdict

This is a good foundation if it is treated as a high-quality blueprint, not as proof that the system is production-ready. The domain primitives are correct: state machines, ledger, outbox, idempotency, webhooks, settlements, reconciliation, failure modes, and runbooks are the right senior-level topics.

The original weak point was distributed consistency. Several flows implied that network-separated services could update payment state, ledger state, and events safely without a formal transaction or saga. That is an assumption, not an architecture. The docs now make the safer implementation posture explicit: modular monolith first for money-critical paths, extract async workers and services later.

---

## What Is Strong

| Area | Assessment |
|------|------------|
| Domain model | Strong. Orders, attempts, payments, refunds, settlements, ledger, webhooks, audit, risk, and reconciliation are the right entities. |
| State machines | Strong. Explicit states and invalid-transition rejection are necessary for payments. |
| Ledger thinking | Strong conceptually. Double-entry, append-only corrections, and reconciliation are the right approach. |
| Outbox pattern | Strong. Correctly avoids DB-write/event-publish dual writes. |
| Webhooks | Strong. Retry, dead-lettering, replay, HMAC signatures, and delivery attempts are all useful. |
| Failure modes | Strong coverage. The docs think beyond happy paths. |
| Testing strategy | Strong direction. State machine, ledger, integration, contract, chaos, and load tests are appropriate. |
| Portfolio value | High, if implemented with working flows and tests instead of only diagrams. |

---

## What Was Assumption-Heavy

| Assumption | Why It Was Risky | Corrected Direction |
|------------|------------------|---------------------|
| Payment service can call Ledger service synchronously, then commit payment state separately | Crash after ledger write but before payment commit creates orphan ledger entries | Capture + ledger + audit + outbox share one DB transaction in Phase 1 |
| Redis-only idempotency is enough | Redis loss can allow duplicate money-changing operations | Add durable Postgres idempotency records for capture/refund/settlement writes |
| Refund ledger entries can be written on refund creation | Failed refund would need correction and creates misleading money movement | Write refund ledger entries only after gateway confirms `processed` |
| "Cursor pagination" without a cursor | Timestamp-only pagination can skip/duplicate rows with same timestamp | Add opaque cursor using `(created_at, id)` |
| Ledger DB constraint enforces transaction balance directly on entry rows | A per-row check cannot enforce multi-row debit/credit balance | Add `ledger_transactions` header and service-level validation |
| Microservices from day one are the senior choice | Distributed systems can hide inconsistency behind diagrams | Start modular monolith, extract after invariants are tested |

---

## Non-Negotiable Invariants

1. A captured payment must have exactly one balanced ledger transaction.
2. A processed refund must have exactly one balanced refund ledger transaction.
3. A settlement item must reference a real captured payment.
4. A payment cannot be settled twice.
5. A payment cannot be refunded beyond captured amount minus already processed and pending refunds.
6. Every money-changing POST must be idempotent across process restarts and Redis loss.
7. Every state mutation must write an audit event and an outbox event in the same transaction.
8. Ledger rows are append-only. Corrections are new transactions.
9. Webhook delivery is at-least-once. Consumers must deduplicate by `event_id`.
10. Reconciliation must be able to explain every mismatch with a reason code.

---

## Recommended Build Strategy

Build the core as one Go backend first:

1. Implement merchants, API keys, auth, and idempotency records.
2. Implement order and payment state machines with exhaustive tests.
3. Implement ledger transactions and entries as an internal package.
4. Implement capture as one PostgreSQL transaction: lock payment, validate transition, insert ledger transaction, update payment, write audit event, write outbox event, complete idempotency record.
5. Implement outbox relay as the first separate worker.
6. Add refunds with pending reservation and processed-time ledger reversal.
7. Add webhooks and settlement only after capture/refund invariants are passing in integration tests.

Do not start by creating nine deployable services. That will slow the project and make correctness harder to prove.

---

## Review Standard For Future Changes

Every new doc or implementation change should answer these questions:

1. What is the exact transaction boundary?
2. What happens if the process crashes after each write?
3. Is the operation idempotent after Redis loss?
4. What ledger entries are created, and do they balance?
5. What event is emitted, and is it emitted through the outbox?
6. What reconciliation rule detects a broken implementation?
7. What test proves the invariant?

If a design cannot answer these, it is not ready to implement.

---

## Advanced track quality bar

If you choose the advanced distributed track, the bar is higher:

1. Every cross-service command must be idempotent and replay-safe.
2. Every saga must have explicit compensation semantics.
3. Every event schema change must pass compatibility and consumer-contract gates.
4. Every hold/release/commit flow must be reconciled against payouts.
5. Every DR drill must produce artifacts: RTO, RPO, replay duration, recon mismatch count.
