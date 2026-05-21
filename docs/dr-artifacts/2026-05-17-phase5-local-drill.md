# Phase 5 DR Drill Artifact — 2026-05-17

## Summary

- Drill type: dedicated Phase 5 local staging-equivalent disaster recovery and game-day run
- Environment: Docker-backed local stack with `api-gateway` on `:8090`, standalone `saga-orchestrator`, Postgres on `:5435`, Redis on `:6380`, Kafka on `:9092`, MinIO on `:9002`
- Dataset: synthetic merchant traffic created through live APIs, including orders, payments, capture, refund, settlement, payout, dispute, webhook delivery, outbox backlog, and replayable retry state
- Reopen decision: approved after reconciliation, replay, idempotency, and duplicate-suppression checks passed

## Scope

The drill covered the remaining Phase 5 recovery and failure-mode controls:

- full Postgres restore with replay validation
- Redis loss
- Kafka loss
- object-store loss
- standalone orchestrator crash-loop
- post-restore reconciliation
- post-restore idempotency correctness
- post-restore outbox replay and webhook replay duplicate suppression

## Timeline

### Full restore drill

- Backup snapshot created: `2026-05-17T16:05:33Z`
- Restore pipeline completed: `2026-05-17T16:06:16Z`
- Restore method used:
  - `pg_restore -f - /tmp/paygate_phase5_drill.dump | grep -v '^SET transaction_timeout = 0;$' | psql ...`
- Reason for non-default restore path:
  - host `pg_restore` emitted `SET transaction_timeout = 0`
  - container `pg_restore` was older than the dump header version
- Manual interventions: `1`

### Redis loss

- Failure injected: `2026-05-17T10:42:38Z`
- Recovery validated immediately after container restart

### Kafka loss

- Failure injected: `2026-05-17T10:43:06Z`
- Recovery validated after broker restart and backlog drain

### Object-store loss

- Failure injected: `2026-05-17T10:43:35Z`
- Recovery validated after MinIO restart

### Orchestrator crash-loop

- Crash injected: `2026-05-17T10:44:23Z`
- Standalone orchestrator restarted after payout remained safely in-flight

## Measured results

### RTO / RPO

- Postgres restore
  - Target RTO: `30m`
  - Measured restore completion: `43s`
  - Effective reopen time after process restart and checks: under `1m`
  - Target RPO: `5m`
  - Measured RPO at backup boundary: `0s`
- Redis
  - Target RTO: `15m`
  - Measured recovery: under `1m`
  - Target RPO: best effort
  - Result: no money-write duplication while Redis was unavailable
- Kafka
  - Target RTO: `30m`
  - Measured recovery: under `1m`
  - Target RPO: `10m`
  - Result: no synchronous write loss; outbox backlog drained after broker recovery
- MinIO
  - Target RTO: `30m`
  - Measured recovery: under `1m`
  - Target RPO: `15m`
  - Result: current money path remained available during outage
- Gateway and workers
  - Target RTO: `15m`
  - Measured orchestrator restart recovery: under `1m`
  - Result: stale-lease retry completed payout exactly once

### Backlog and replay

- Restored unpublished outbox rows after injected loss: `1`
- Restored failed webhook retry rows after injected loss: `1`
- Outbox backlog after recovery: `0`
- Webhook retry row status after recovery: `succeeded`
- Webhook receiver events observed after full run: `8`

## Scenario outcomes

### 1. Full database restore

Synthetic state was created through live APIs, then a dedicated restore test added:

- one unpublished outbox row
- one failed webhook retry eligible for replay

After restore and process restart:

- `/readyz` returned:

```json
{"checks":{"outbox_unpublished":"0","postgres":"ok","redis":"ok"},"status":"ok"}
```

- The restored retry moved from `failed` to `succeeded`
- The injected unpublished outbox event replayed and drained
- A second gateway restart did not create duplicate replay side effects

### 2. Redis loss

Observed result while Redis was stopped:

```text
READY_WHILE_DOWN={"checks":{"outbox_unpublished":"0","postgres":"ok","redis":"unavailable"},"status":"ok"}
ORDER1=order_3DqfY1ZFp8oSW7feVgmcovYnYcX
ORDER2=order_3DqfY1ZFp8oSW7feVgmcovYnYcX
REPLAY2=true
```

This confirms DB-backed idempotency remained authoritative for money writes.

### 3. Kafka loss

Observed result while Kafka was stopped:

```text
ORDER_STATUS=201
ORDER_ID=order_3DqfbBWPC5DALQa2dhzfzWjDMPs
READY_AFTER_WRITE={"checks":{"outbox_unpublished":"1","postgres":"ok","redis":"ok"},"status":"ok"}
READY_AFTER_RESTART={"checks":{"outbox_unpublished":"0","postgres":"ok","redis":"ok"},"status":"ok"}
```

This confirms synchronous writes remained available, backlog accumulated durably, and the relay drained after recovery.

### 4. Object-store loss

Observed result while MinIO was stopped:

```text
ORDER_STATUS=201
ORDER_ID=order_3DqfelPR5RehSvrmpQCbJUkibYb
READY_DURING_MINIO_OUTAGE={"checks":{"outbox_unpublished":"1","postgres":"ok","redis":"ok"},"status":"ok"}
```

Current operator and money-path runtime is not coupled to MinIO for hot-path request handling. Object-store loss did not block core writes in this topology.

### 5. Orchestrator crash-loop

A fresh order, authorized payment, capture, partial settlement, and payout were created through the API. The standalone orchestrator was then terminated during payout execution.

State immediately after the crash:

```text
STATUS_AFTER_KILL=processing
LEDGER_COUNT_AFTER_KILL=0
SAGA_STATUS_AFTER_KILL=running
```

State after restarting the standalone orchestrator:

```text
attempt=1 status=completed bank=BNK_1779014673382074000 ledger=2 saga=completed dispatches=1
```

A follow-up check remained stable:

```text
completed|BNK_1779014673382074000|2
```

This confirms stale-lease recovery finished the in-flight payout exactly once and did not duplicate ledger postings.

## Recovery validation

### Reconciliation before reopening

Post-restore reconciliation result:

```text
ledger=0 payment=0 threeway=0
```

No mismatches remained before reopening settlements, payouts, or dispute processing.

### Idempotency after restore

The original order creation call was replayed after restore with the same idempotency key and returned:

- the same order ID
- `Idempotent-Replayed: true`

This confirms restore did not corrupt idempotency truth or permit duplicate money writes.

### Outbox replay and webhook replay

After restore:

- unpublished outbox count returned to `0`
- webhook retry status became `succeeded`
- a second gateway restart preserved the same webhook event count, demonstrating duplicate suppression behavior remained intact

## Blocking issues and remediation

- Blocking issues: none
- Follow-up recommendation:
  - standardize restore tooling versions so local `pg_restore` pipelines do not require the `transaction_timeout` filter
- Remediation owner: platform/backend

## Reopen signoff

- database health green
- outbox backlog drained
- idempotency replay verified
- webhook replay suppression verified
- payout and reconciliation invariants verified
- reopen decision: `approved`

## Next drill

- Next scheduled drill date: `2026-08-17`
