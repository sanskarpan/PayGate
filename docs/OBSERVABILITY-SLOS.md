# Observability SLOs

PayGate tracks operator-facing service-level indicators directly from runtime
Prometheus metrics and exposes the most important slices in the dashboard
observability screen.

## Authorization SLOs

Gateway authorization metrics are emitted as:

- `paygate_gateway_authorizations_total{method,provider,outcome}`
- `paygate_gateway_authorization_duration_seconds_{bucket,sum,count}{method,provider}`

Recommended operator targets:

- card / simulator success rate: `>= 98%`
- UPI / simulator success rate: `>= 97%`
- mean authorization latency by method/provider: `< 750ms`
- error outcome rate by method/provider: `< 1%`

Success is defined as:

- `authorized`
- `requires_action`

Non-success outcomes are:

- `declined`
- `error`

## Webhook Delivery SLOs

Webhook signature-mode adoption and final delivery outcomes are emitted as:

- `paygate_webhook_signature_deliveries_total{signature_mode,status}`

Recommended operator targets:

- success rate by signature mode: `>= 99%`
- dead-letter rate by signature mode: `< 0.1%`

## Dashboard Usage

The dashboard observability page now shows:

- per-method and per-provider authorization success and average latency
- webhook delivery counts by signature mode
- existing aggregate counters for orders, payments, refunds, disputes, payouts,
  and outbox backlog

## Alerting Guidance

Suggested alerts:

- authorization success rate below threshold for any method/provider for 10m
- average authorization latency above threshold for any method/provider for 10m
- webhook dead-letter volume above threshold by signature mode for 15m
- outbox backlog above operational threshold for 10m
