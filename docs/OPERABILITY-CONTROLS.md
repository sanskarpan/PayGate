# Operability And Rollout Controls

## Correlation strategy

Every command, event, webhook, payout, and ledger mutation should carry:

- `request_id`
- `correlation_id`
- `causation_id`
- `merchant_id`
- business object ID (`order_id`, `payment_id`, `payout_id`, `hold_id`, `saga_id`)

Tracing order of precedence:

1. inbound request ID
2. saga correlation ID
3. outbox event ID
4. downstream webhook event ID

## SLOs and error budgets

- command dispatch latency
  - target: p95 < 2s
- orchestration lag
  - target: p95 < 5s from command creation to handler lease
- payout callback latency
  - target: p95 < 30s for simulated rail callbacks
- replay completion time
  - target: p95 < 2m
- outbox publish latency
  - target: p95 < 5s

Monthly error budget: 0.1% for money-path state divergence, 1% for non-money-path async lag.

## Dashboards

- saga backlog
- stale lease count
- duplicate command count
- schema publish version spread
- consumer observed schema versions
- hold aging and expired hold backlog
- payout callback lag
- replay queue depth

## Alerts

- stale saga lease older than 2x lease TTL
- dead-letter or poison command count > 0
- deprecated schema version still active past deadline
- hold expiry sweeper backlog above threshold
- payout callback signature failures above threshold
- replay queue lag above threshold

## Feature flags and kill-switches

- `SAGA_DISPATCH_ENABLED`
- `PAYOUT_CALLBACKS_ENABLED`
- `WEBHOOK_REPLAY_ENABLED`
- `SCHEMA_ACTIVATION_ENABLED`
- `EXTRACTED_PAYOUT_SHADOW_MODE`

## Shadow mode diffing

When shadow mode is enabled compare:

- saga terminal state
- payout terminal state
- number of ledger postings
- summed debit/credit net by account
- emitted outbox event set

Any mismatch should:

- keep monolith ownership
- emit audit evidence
- open a remediation issue

## Audit requirements

The following operator actions must be written to immutable audit logs:

- saga replay
- saga force replay
- future saga override / force-complete
- payout callback disable / enable
- schema activation
- schema rollback
- webhook replay
