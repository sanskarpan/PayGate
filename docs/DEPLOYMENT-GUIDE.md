# Deployment Guide

## Container images

One Dockerfile builds every Go service; pick the binary with `SERVICE`:

```bash
docker build --build-arg SERVICE=api-gateway -t paygate/api-gateway .
docker build --build-arg SERVICE=saga-orchestrator -t paygate/saga-orchestrator .
docker build -t paygate/dashboard dashboard/
```

The Go images are distroless and run as a non-root user. `schemas/` is baked in
because `api-gateway` bootstraps the event schema registry from those fixtures
at startup and fails to start without them.

The dashboard build needs egress to `fonts.googleapis.com`: the layout uses
`next/font/google`, which downloads the font files at build time. Builds on a
restricted network fail with `FetchError` unless that host is reachable.

## Local stack

Infrastructure only (Postgres, Redis, Kafka, MinIO, Prometheus, Alertmanager, Grafana):

```bash
docker compose up -d
```

The whole product, containerised, on top of it:

```bash
docker compose -f docker-compose.app.yml up -d --build
# API       http://localhost:8090
# Dashboard http://localhost:3001
```

Override `PAYGATE_API_PORT` / `PAYGATE_DASHBOARD_PORT` if those ports are taken.

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

## Kubernetes

Manifests live in [`deployments/k8s`](../deployments/k8s) and are applied in name
order. They are vendor-neutral; the ingress annotations assume nginx.

```bash
kubectl apply -f deployments/k8s/00-namespace.yaml
# Create paygate-secrets out of band — 11-secret.example.yaml is a template only.
kubectl apply -f deployments/k8s/10-config.yaml
kubectl -n paygate create configmap paygate-migrations --from-file=migrations/
kubectl apply -f deployments/k8s/20-migrate-job.yaml
kubectl -n paygate wait --for=condition=complete job/paygate-migrate --timeout=5m
kubectl apply -f deployments/k8s/30-api-gateway.yaml -f deployments/k8s/40-dashboard.yaml
kubectl apply -f deployments/k8s/50-ingress.yaml
```

Things that are easy to get wrong:

- **`DASHBOARD_ORIGIN` must equal the origin the browser uses for the dashboard.**
  The session cookie is set by the API on login; if the origins disagree the
  browser will not send it back and every page bounces to the login screen.
- **`API_BASE_URL` must be the public API URL**, because the browser posts the
  login form straight to it. Server-side rendering uses `API_INTERNAL_BASE_URL`
  instead, which should point at the in-cluster Service.
- **`TRUSTED_PROXY_CIDRS` must list only your ingress/pod ranges.** Anything
  wider lets clients spoof `X-Forwarded-For`, which feeds rate limiting and risk
  scoring.
- **Keep pods on the timezone the dashboard formats in** (`TZ` in the ConfigMap).
- With `APP_ENV=production` the API refuses to start unless the session and
  payout-rail secrets are set to non-default values of sufficient length.

## Recommended rollout order

1. Apply migrations and wait for the job to complete.
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
