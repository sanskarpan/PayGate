# PayGate — Runbook

> Operational procedures for common scenarios. What to do when things go wrong.

---

## 1. Incident response playbooks

### P1: Ledger imbalance detected

**Symptoms**: Reconciliation worker fires `recon.mismatch.detected` with type `MISSING_LEDGER_ENTRY` or `AMOUNT_MISMATCH`.

**Impact**: Financial data integrity compromised. Settlement accuracy at risk.

**Steps**:
1. **Halt settlements** — pause the settlement CronJob immediately
   ```bash
   kubectl patch cronjob settlement-batch -p '{"spec":{"suspend":true}}'
   ```
2. **Identify affected payments** — query the reconciliation_mismatches table
   ```sql
   SELECT * FROM reconciliation_mismatches
   WHERE batch_id = '{latest_batch_id}' AND severity = 'critical'
   ORDER BY created_at;
   ```
3. **Cross-reference with audit log** — find the capture event
   ```sql
   SELECT * FROM audit_events
   WHERE resource_type = 'payment' AND resource_id = '{payment_id}'
   AND action = 'payment.captured';
   ```
4. **Check outbox** — was the event published?
   ```sql
   SELECT * FROM outbox
   WHERE aggregate_type = 'payment' AND aggregate_id = '{payment_id}';
   ```
5. **Create manual correction** — use admin API to insert compensating ledger entries
6. **Re-run reconciliation** — verify the fix
7. **Resume settlements** after verification
   ```bash
   kubectl patch cronjob settlement-batch -p '{"spec":{"suspend":false}}'
   ```
8. **Post-incident**: file a bug for the root cause

---

### P2: Webhook delivery exhausted (dead-lettered)

**Symptoms**: `webhook.delivery.exhausted` alert fires. Merchant is not receiving events.

**Steps**:
1. **Check delivery attempt history**
   ```sql
   SELECT attempt_number, response_status, error, created_at
   FROM webhook_delivery_attempts
   WHERE webhook_event_id = '{event_id}'
   ORDER BY attempt_number;
   ```
2. **Diagnose**:
   - All `5xx` → merchant's server is down. Notify merchant.
   - All `timeout` → merchant's endpoint is slow. Notify merchant.
   - `dns_resolution_failed` → domain is invalid. Notify merchant.
   - Mix of statuses → intermittent issue. May self-resolve.
3. **Notify merchant** via email or dashboard notification
4. **Once merchant confirms fix**: replay the event
   ```bash
   curl -X POST https://api.paygate.dev/v1/webhooks/events/{event_id}/replay \
     -u {ops_api_key}:{secret}
   ```

---

### P2: Outbox relay backlog

**Symptoms**: Prometheus metric `outbox_unpublished_count > 1000` for > 5 minutes.

**Steps**:
1. **Check relay process health**
   ```bash
   kubectl get pods -l app=outbox-relay
   kubectl logs -l app=outbox-relay --tail=100
   ```
2. **Check Kafka health**
   ```bash
   kubectl exec -it kafka-0 -- kafka-topics.sh --describe --bootstrap-server localhost:9092
   ```
3. **If relay is crashed**: it will auto-restart (Kubernetes). Monitor backlog clearing.
4. **If Kafka is unhealthy**: check broker logs, disk space, replication status.
5. **If backlog is clearing but slowly**: consider scaling relay replicas temporarily.
6. **Monitor**: the outbox backlog should clear within minutes of the issue resolving. Events are published in order.

---

### P2: High payment failure rate

**Symptoms**: `payment_failure_rate > 10%` alert over 5-minute window.

**Steps**:
1. **Check gateway proxy health** — is the simulator misconfigured?
   ```bash
   curl https://gateway-proxy.internal/health
   curl https://gateway-proxy.internal/config  # check current mode
   ```
2. **Check recent failure reasons**
   ```sql
   SELECT error_code, COUNT(*) as cnt
   FROM payments
   WHERE status = 'failed' AND created_at > NOW() - INTERVAL '30 minutes'
   GROUP BY error_code ORDER BY cnt DESC;
   ```
3. **If `GATEWAY_TIMEOUT`**: check network connectivity to gateway proxy
4. **If `INSUFFICIENT_FUNDS`**: normal — buyer-side issue, not platform issue
5. **If `GATEWAY_ERROR`**: check gateway proxy logs for root cause
6. **If rate is spiking for one merchant**: check if they're under attack (velocity spike). Consider enabling risk hold.

---

## 2. Common operational procedures

### Rotate a merchant's API key

1. Merchant creates a new key via dashboard or API
2. Merchant updates their integration to use the new key
3. Merchant revokes the old key via dashboard or API
4. Old key returns 401 immediately after revocation

If the merchant locked themselves out (revoked without migrating):
```bash
# Ops generates a new key via admin API
curl -X POST https://api.paygate.dev/admin/merchants/{merchant_id}/keys \
  -u {admin_key}:{secret} \
  -d '{"scope": "write", "mode": "live"}'
```

### Rotate a webhook secret

1. Merchant creates a new secret for the subscription
2. PayGate signs with BOTH old and new secret during grace period (24h)
3. After grace period, old secret is removed
4. Merchant should verify using the new secret

### Place a settlement hold

```sql
UPDATE settlements SET status = 'on_hold', hold_reason = 'fraud_investigation'
WHERE merchant_id = '{merchant_id}' AND status = 'created';

-- Also prevent new settlements:
UPDATE merchants SET settings = jsonb_set(settings, '{settlement_hold}', 'true')
WHERE id = '{merchant_id}';
```

### Release a settlement hold

```sql
UPDATE merchants SET settings = jsonb_set(settings, '{settlement_hold}', 'false')
WHERE id = '{merchant_id}';

-- Held settlements will be picked up in the next batch
UPDATE settlements SET status = 'created', hold_reason = NULL
WHERE merchant_id = '{merchant_id}' AND status = 'on_hold';
```

### Re-run a failed settlement batch

Settlement batches are idempotent by design. Safe to re-run:
```bash
kubectl create job settlement-rerun --from=cronjob/settlement-batch
```

### Investigate a reconciliation mismatch

```sql
-- 1. Get latest batch
SELECT * FROM reconciliation_batches ORDER BY started_at DESC LIMIT 1;

-- 2. Get mismatches
SELECT * FROM reconciliation_mismatches WHERE batch_id = '{batch_id}' AND resolved = false;

-- 3. For each mismatch, cross-reference:
-- Payment record
SELECT * FROM payments WHERE id = '{payment_id}';

-- Ledger entries
SELECT * FROM ledger_entries WHERE source_type = 'payment' AND source_id = '{payment_id}';

-- Settlement item
SELECT * FROM settlement_items WHERE payment_id = '{payment_id}';

-- 4. Resolve
UPDATE reconciliation_mismatches
SET resolved = true, resolved_by = 'ops_user', resolved_at = NOW()
WHERE id = '{mismatch_id}';
```

---

## 3. Monitoring dashboards

### Payment funnel dashboard
- Orders created/min
- Payment attempts/min
- Authorization success rate (%)
- Capture success rate (%)
- Average time: order → capture
- Payment method distribution

### Webhook delivery dashboard
- Events generated/min
- Deliveries attempted/min
- First-attempt success rate (%)
- Retry rate (%)
- Dead-letter count (should be 0)
- Average delivery latency (event creation → first successful delivery)
- Top failing merchant endpoints

### Settlement dashboard
- Payments awaiting settlement
- Settlements created today
- Total gross/net amounts
- Average settlement cycle time
- Settlement holds count

### Infrastructure dashboard
- Service health (all pods green)
- PostgreSQL: connections, query latency, replication lag
- Kafka: consumer lag per group, partition count, broker health
- Redis: memory usage, connection count, eviction rate
- Outbox: unpublished entry count, relay throughput
- API gateway: request rate, error rate, p50/p95/p99 latency

---

## 4. Backup and recovery

### Database backups
- **Continuous**: WAL archiving to S3 (point-in-time recovery)
- **Daily**: pg_dump of all schemas, uploaded to S3 with 30-day retention
- **Weekly**: full base backup

### Recovery procedure
1. Stop all services
2. Restore PostgreSQL from WAL archive to target timestamp
3. Verify ledger balance (run reconciliation)
4. Restart services
5. Outbox relay will catch up on any events created after restore point

### Kafka topic recovery
- Topics retain data for 7-30 days (configurable)
- Consumer groups track offsets — services resume from where they left off
- If a topic is lost: recreate and replay from outbox table (outbox entries retained for 7 days after publishing)

---

## 5. Advanced operations (optional track)

### Recover a stuck saga
1. Inspect `saga_instances` and `saga_steps` for failed step and command ID.
2. Confirm whether command was already processed via `processed_commands`.
3. Trigger `POST /v1/sagas/{saga_id}/replay`.
4. Verify no duplicate ledger transaction was created.

### Perform schema rollout safely
1. Register new schema in draft.
2. Run producer compatibility and consumer contract checks.
3. Activate dual-publish (`v1` + `v2`).
4. Confirm consumer migration dashboard reaches 100%.
5. Disable old version and mark deprecated.

### DR drill checklist
1. Simulate region failure.
2. Restore DB to target point-in-time.
3. Replay outbox backlog and verify Kafka consumer offsets.
4. Run full reconciliation.
5. Resume settlements only after zero critical mismatches.
