# ADR 006: First Extracted Flow Is Payout Orchestration

## Status

Accepted

## Context

Phase 5 needs one core flow extracted behind durable command and event contracts without changing the merchant-facing API. Payout orchestration is the best first candidate because:

- it already has a bounded state machine
- it already uses ledger holds
- it already emits a compact event set
- it benefits from exactly-once callback processing and replay controls

## Decision

The first extracted flow will be payout orchestration.

- `api-gateway` remains the merchant-facing API
- `saga-orchestrator` owns command leasing and replay decisions
- `payout-service` owns payout state
- `ledger-service` owns money posting

## Consequences

Positive:

- exercises all core distributed systems concerns in one flow
- limits blast radius compared with extracting authorization or settlement first
- provides a clean shadow-mode candidate

Tradeoffs:

- requires coordination between payout, ledger, and webhook surfaces
- increases complexity in callback and replay testing before full extraction
