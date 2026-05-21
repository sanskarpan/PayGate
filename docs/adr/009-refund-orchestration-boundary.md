# ADR 009: Refund Orchestration Boundary

## Decision

Refund orchestration is forward-only after the gateway accepts the refund. Compensation is modeled as ledger correction and operator-visible remediation, not a guaranteed external reversal.

## Why

- refund gateways are not universally reversible
- accounting still must remain correct even when the external rail is not

## Consequence

Refund flows must persist enough state to replay and inspect, but failed post-gateway steps are resolved with durable corrective actions rather than pretending the external refund can always be undone.
