# ADR 010: Settlement Finalization Boundary

## Decision

Settlement finalization is treated as a derived accounting phase. Once a settlement batch is processed, payout orchestration owns the asynchronous transfer lifecycle while the settlement record remains immutable except for hold metadata.

## Why

- changing finalized settlement math after payout initiation risks breaking reconciliation
- payout returns/reversals should be modeled as corrective movements, not silent settlement mutation

## Consequence

Settlement records provide the stable base amount. Payout rail callbacks and saga replay operate on payout state and ledger corrections instead of rewriting historical settlement totals.
