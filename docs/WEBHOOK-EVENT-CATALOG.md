# Webhook Event Catalog

This catalog maps webhook/event subjects to their canonical JSON schema fixtures under [`schemas/events/`](/Users/sanskar/dev/PayGate/schemas/events).

## Payment lifecycle

- `payment.authorized`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payment.authorized/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payment.authorized/1.0.0.sample.json)
- `payment.captured`: [schema v1.0.0](/Users/sanskar/dev/PayGate/schemas/events/payment.captured/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payment.captured/1.0.0.sample.json)
- `payment.failed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payment.failed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payment.failed/1.0.0.sample.json)
- `payment.auto_refunded`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payment.auto_refunded/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payment.auto_refunded/1.0.0.sample.json)
- `order.created`: [schema](/Users/sanskar/dev/PayGate/schemas/events/order.created/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/order.created/1.0.0.sample.json)
- `order.paid`: [schema](/Users/sanskar/dev/PayGate/schemas/events/order.paid/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/order.paid/1.0.0.sample.json)

## Refunds

- `refund.created`: [schema](/Users/sanskar/dev/PayGate/schemas/events/refund.created/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/refund.created/1.0.0.sample.json)
- `refund.processed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/refund.processed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/refund.processed/1.0.0.sample.json)
- `refund.failed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/refund.failed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/refund.failed/1.0.0.sample.json)

## Settlements and payouts

- `settlement.processed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/settlement.processed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/settlement.processed/1.0.0.sample.json)
- `payout.created`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payout.created/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.created/1.0.0.sample.json)
- `payout.initiated`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payout.initiated/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.initiated/1.0.0.sample.json)
- `payout.completed`: [schema v1.0.0](/Users/sanskar/dev/PayGate/schemas/events/payout.completed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.completed/1.0.0.sample.json)
- `payout.failed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payout.failed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.failed/1.0.0.sample.json)
- `payout.returned`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payout.returned/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.returned/1.0.0.sample.json)
- `payout.reversed`: [schema](/Users/sanskar/dev/PayGate/schemas/events/payout.reversed/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/payout.reversed/1.0.0.sample.json)

## Disputes and risk

- `dispute.created`: [schema](/Users/sanskar/dev/PayGate/schemas/events/dispute.created/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/dispute.created/1.0.0.sample.json)
- `dispute.updated`: [schema](/Users/sanskar/dev/PayGate/schemas/events/dispute.updated/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/dispute.updated/1.0.0.sample.json)
- `dispute.accepted`: [schema](/Users/sanskar/dev/PayGate/schemas/events/dispute.accepted/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/dispute.accepted/1.0.0.sample.json)
- `dispute.won`: [schema](/Users/sanskar/dev/PayGate/schemas/events/dispute.won/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/dispute.won/1.0.0.sample.json)
- `dispute.lost`: [schema](/Users/sanskar/dev/PayGate/schemas/events/dispute.lost/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/dispute.lost/1.0.0.sample.json)
- `risk.alert`: [schema](/Users/sanskar/dev/PayGate/schemas/events/risk.alert/1.0.0.schema.json), [sample](/Users/sanskar/dev/PayGate/schemas/events/risk.alert/1.0.0.sample.json)

## Notes

- Every fixture includes the required envelope fields enforced in CI: `event_id`, `event_type`, `occurred_at`, `correlation_id`, `causation_id`, `schema_version`, `merchant_id`, and `payload`.
- Additive schema evolution should append a new version in the same subject directory and update samples at the same time.
