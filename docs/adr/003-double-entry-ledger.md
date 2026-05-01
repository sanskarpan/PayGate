# ADR-003: Double-Entry Ledger Model

**Status:** Accepted
**Date:** 2026-05-01

## Context

PayGate moves money between parties (customers, merchants, platform). We need an auditable financial record that can reconstruct balances at any point in time, support reconciliation, and detect accounting bugs (unbalanced entries).

## Decision

Implement a **double-entry ledger** where every financial event produces a set of `Entry` rows that satisfy the invariant:

```
SUM(debit_amount) == SUM(credit_amount)  for any transaction
```

Account codes used:
| Code | Meaning |
|---|---|
| `CUSTOMER_RECEIVABLE` | Amount owed by the customer |
| `MERCHANT_PAYABLE` | Amount owed to the merchant |
| `PLATFORM_FEE_REVENUE` | Platform's retained fee |

Each ledger transaction references `(source_type, source_id)` (e.g., `payment` / `pay_xxx`) and is append-only — rows are never updated or deleted.

The service validates balance before inserting:
```go
func ValidateEntries(entries []Entry) error {
    var debit, credit int64
    for _, e := range entries { debit += e.DebitAmount; credit += e.CreditAmount }
    if debit != credit { return ErrUnbalancedEntries }
    return nil
}
```

## Consequences

**Positive:**
- Any account balance can be computed as `SUM(credit) - SUM(debit)` at any point in time
- Bugs in entry construction are caught at write time, not at reconciliation time
- Append-only design makes auditing and compliance straightforward

**Negative:**
- More complex than a simple payment amount column; requires 2–4 rows per transaction
- Balance queries require aggregation; an indexed materialized view may be needed at scale

## Alternatives Considered

- **Single-column balance update:** Simple but non-auditable, prone to race conditions on concurrent captures, and cannot reconstruct history
- **Event sourcing entire ledger:** Technically correct but adds significant complexity for queries and projections
