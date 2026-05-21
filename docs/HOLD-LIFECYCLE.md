# Ledger Hold Lifecycle

## Hold classes

- Authorization hold
  - temporary reservation before a final ledger posting
- Reserve hold
  - finance or ops reserve against clearing balance
- Compliance hold
  - block funds while review is in progress
- Payout hold
  - explicit block on payout initiation
- Dispute hold
  - reserve tied to a dispute workflow

## State semantics

- `active`
  - contributes to reserved balance
  - blocks payoutability when applied to settlement clearing
- `released`
  - idempotent terminal state
  - no ledger posting created
- `committed`
  - terminal state with exactly one final ledger posting
- `expired`
  - terminal state set by sweeper or explicit operator action

## Traceability

Each hold should be traceable through:

- business reference
  - `source_type`
  - `source_id`
- ledger reference
  - commit source type `ledger_hold_commit`
  - commit source ID equal to `hold_id`
