# Disaster Recovery Plan

## DR scope

Stateful dependencies in scope:

- Postgres
- Kafka
- Redis
- MinIO
- schema registry state (`paygate_schema`)
- dashboard session state
- secrets and configuration
- deployment manifests and service topology

## Target RTO / RPO

- Postgres
  - RTO: 30 minutes
  - RPO: 5 minutes
- Kafka
  - RTO: 30 minutes
  - RPO: 10 minutes
- Redis
  - RTO: 15 minutes
  - RPO: best effort, idempotency fallback allowed
- MinIO
  - RTO: 30 minutes
  - RPO: 15 minutes
- Gateway and workers
  - RTO: 15 minutes
  - RPO: stateless redeploy

## Backup cadence and retention

- Postgres
  - full snapshot daily
  - WAL / PITR stream continuously
  - retain 35 days
- Kafka
  - topic config backup daily
  - consumer group offsets exported every 5 minutes
- Redis
  - snapshot every 15 minutes
  - not relied upon as the only source of idempotency truth
- MinIO
  - versioned bucket replication hourly
- Secrets and manifests
  - encrypted export on every approved change

## Restore playbooks

### Single-service restore

- restore dependency snapshot
- restore config / secret material
- run health checks
- replay backlog
- verify reconciliation before reopening affected flows

### Full-region restore

- recreate infrastructure from manifests
- restore Postgres
- restore Kafka metadata and topics
- restore MinIO objects
- restore Redis snapshot
- redeploy gateway, workers, and dashboard
- verify:
  - outbox drains
  - webhook replay suppression still works
  - payout and settlement reconciliation pass

### Partial corruption

- isolate corrupt subsystem
- freeze affected writes
- restore from last known good snapshot
- compare before/after state and record remediation

## Drill process

For each drill record:

- start and end timestamps
- declared scenario
- services restored
- backlog replay duration
- manual intervention count
- measured RTO and RPO
- reconciliation outcome
- reopen decision

## Reopen checklist

- database health green
- outbox unpublished backlog stable or draining
- idempotency path verified
- webhook duplicate suppression verified
- payout return/reversal invariants verified
- reconciliation completed with acceptable mismatch threshold
- operator signoff recorded

## Artifact checklist

- timeline
- screenshots or logs
- metric snapshots
- unresolved blockers
- remediation owner
- next scheduled drill date

## Recorded drill artifacts

- [2026-05-17 Phase 5 local drill](dr-artifacts/2026-05-17-phase5-local-drill.md)

## Follow-up issue process

- every failed control becomes a tracked issue
- every manual step longer than target becomes a tracked issue
- every mismatch after restore becomes a tracked issue
- issues must include owner, severity, and next verification date
