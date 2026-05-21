# Deployment Guide

## Local stack

Use Docker Compose for Postgres, Redis, Kafka, MinIO, Prometheus, Alertmanager, and Grafana.

```bash
docker compose up -d
```

Start the API gateway:

```bash
go run ./cmd/api-gateway
```

Start the standalone saga worker when extracting orchestration from the gateway:

```bash
SAGA_WORKER_ENABLED=false go run ./cmd/api-gateway &
go run ./cmd/saga-orchestrator
```

Start the dashboard:

```bash
cd dashboard
pnpm install
pnpm dev
```

## Production shape

- `api-gateway`: synchronous API traffic and operator endpoints
- `outbox-relay`: async event publishing
- `webhook-service`: webhook delivery
- `saga-orchestrator`: saga command execution and timeout recovery
- `recon-worker`: reconciliation batches

## Recommended rollout order

1. Apply migrations.
2. Start Postgres, Redis, Kafka, MinIO.
3. Start `outbox-relay`, `webhook-service`, and `saga-orchestrator`.
4. Start `api-gateway`.
5. Start dashboard.
6. Validate `/healthz`, `/readyz`, webhook delivery, and metrics.

## Kill switches

- disable saga dispatch
- disable payout callback acceptance
- disable webhook replay
- freeze schema activation

See [`docs/OPERABILITY-CONTROLS.md`](/Users/sanskar/dev/PayGate/docs/OPERABILITY-CONTROLS.md:1) and [`docs/PHASE5-EXTRACTION-PLAN.md`](/Users/sanskar/dev/PayGate/docs/PHASE5-EXTRACTION-PLAN.md:1).
