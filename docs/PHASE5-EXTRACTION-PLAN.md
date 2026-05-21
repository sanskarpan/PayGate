# Phase 5 Extraction Plan

## Target extraction boundaries

The first extracted flow is payout execution orchestration.

- `api-gateway`
  - merchant-facing API surface
  - authentication, idempotency, request metadata
  - synchronous validation before command creation
- `saga-orchestrator`
  - durable command leasing
  - retry and replay control
  - timeout and compensation decisions
- `payout-service`
  - payout state machine
  - rail callback handling
  - corrective ledger instructions
- `ledger-service`
  - money movement posting
  - hold reservation and release
  - exactly-once posting guarantees
- `webhook-service`
  - subscriber fanout
  - retry lifecycle
  - replay controls

## Ownership model

### Command ownership

- `payout.complete_transfer`
  - emitted by `saga-orchestrator`
  - consumed by `payout-service`
  - source of truth for durable command state: `paygate_sagas`

### Event ownership

- `payout.created`, `payout.initiated`, `payout.completed`, `payout.failed`, `payout.returned`, `payout.reversed`
  - emitted by `payout-service`
  - consumed by `webhook-service`, analytics, and future settlement observers
  - source of truth for business state: `paygate_payouts`
- `ledger_hold_commit`
  - emitted by `ledger-service`
  - source of truth for money movement: `paygate_ledger`

## Data ownership

- `paygate_sagas.*`
  - write authority: orchestrator only
- `paygate_payouts.*`
  - write authority: payout service only
- `paygate_ledger.*`
  - write authority: ledger service only
- `public.outbox`
  - write authority: originating business service in the same transaction as state change
- `paygate_schema.*`
  - write authority: schema governance/admin APIs

## Shadow-mode extraction

- keep merchant-facing API unchanged
- dual-run payout execution in monolith and extracted control path
- compare:
  - payout terminal status
  - bank reference / rail reference
  - ledger entry count and net effect
  - webhook event set
- fail closed:
  - if extracted result diverges, retain monolith ownership and open a reconciliation issue

## Rollout phases

1. Dark launch
   - extracted components deployed
   - no live command ownership
2. Shadow traffic
   - commands mirrored
   - outputs diffed, not committed live
3. Partial traffic
   - explicit merchant allowlist
   - kill-switch required
4. Full cutover
   - extracted service becomes write authority
5. Rollback
   - disable extracted ownership
   - replay pending commands from persisted state

## Rollback conditions

- duplicate ledger postings
- payout callback lag above SLO
- stale saga lease accumulation
- schema rollout mismatch between producer and consumer
- reconciliation delta after payout return/reversal

## Kill-switch expectations

- disable saga dispatch
- disable payout callback acceptance
- disable schema activation
- disable webhook replay
