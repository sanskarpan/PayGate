# ADR 007: Order Orchestration Boundary

## Decision

Orders remain source-of-truth data owned by the order module. Downstream orchestration consumes order IDs and immutable order facts instead of mutating order records across service boundaries.

## Why

- order creation is merchant-facing and latency-sensitive
- payment orchestration depends on stable order facts
- cross-service writes into order state would create ambiguous ownership

## Consequence

Extracted flows read order state through a contract and emit follow-on commands/events, but do not directly write order tables outside the order module.
