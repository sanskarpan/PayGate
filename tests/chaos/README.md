# PayGate Chaos Tests

Chaos tests verify the system degrades gracefully under infrastructure failures.

## Prerequisites

- Docker Compose running (`make infra-up`)
- Toxiproxy: https://github.com/Shopify/toxiproxy
  ```bash
  docker run -d --name toxiproxy \
    --network paygate_default \
    -p 8474:8474 \
    -p 25432:25432 \
    -p 26379:26379 \
    ghcr.io/shopify/toxiproxy
  ```

## Toxiproxy Setup

Configure proxies to sit between PayGate and its dependencies:

```bash
# PostgreSQL proxy (PayGate → toxiproxy:25432 → postgres:5432)
curl -X POST http://localhost:8474/proxies \
  -d '{"name":"postgres","listen":"0.0.0.0:25432","upstream":"postgres:5432","enabled":true}'

# Redis proxy (PayGate → toxiproxy:26379 → redis:6379)
curl -X POST http://localhost:8474/proxies \
  -d '{"name":"redis","listen":"0.0.0.0:26379","upstream":"redis:6379","enabled":true}'
```

## Chaos Scenarios

### 1. DB failure during capture
```bash
# Add latency to simulate slow DB
curl -X POST http://localhost:8474/proxies/postgres/toxics \
  -d '{"name":"latency","type":"latency","attributes":{"latency":5000,"jitter":2000}}'

# Run payment capture — should timeout with 503
curl -X POST http://localhost:8090/v1/payments/$PAY_ID/capture \
  -H "Authorization: Basic $CREDS" \
  -d '{"amount":50000,"currency":"INR"}'

# Remove toxic
curl -X DELETE http://localhost:8474/proxies/postgres/toxics/latency
```

### 2. DB complete failure
```bash
# Disable postgres proxy
curl -X POST http://localhost:8474/proxies/postgres/toxics \
  -d '{"name":"down","type":"bandwidth","attributes":{"rate":0}}'

# Health check should report postgres unavailable
curl http://localhost:8090/readyz

# Re-enable
curl -X DELETE http://localhost:8474/proxies/postgres/toxics/down
```

### 3. Redis failure (idempotency graceful degradation)
```bash
# Kill redis proxy
curl -X DELETE http://localhost:8474/proxies/redis

# POST operations should fall back to DB-only idempotency
curl -X POST http://localhost:8090/v1/orders \
  -H "Authorization: Basic $CREDS" \
  -H "Idempotency-Key: chaos-test-001" \
  -d '{"amount":50000,"currency":"INR","receipt":"chaos_test"}'

# Second request with same key should still return cached response
# (from Postgres, not Redis)
curl -X POST http://localhost:8090/v1/orders \
  -H "Authorization: Basic $CREDS" \
  -H "Idempotency-Key: chaos-test-001" \
  -d '{"amount":50000,"currency":"INR","receipt":"chaos_test"}'
```

### 4. Webhook endpoint slow/down
The gateway simulator already supports `timeout` and `slow` modes.
```bash
# Switch gateway to timeout mode
curl -X POST http://localhost:8090/v1/gateway/scenarios \
  -H "Authorization: Basic $CREDS" \
  -d '{"mode":"timeout"}'

# Create payment — should be marked failed after timeout
# Webhook delivery to a slow endpoint is simulated by
# setting up a mock endpoint with artificial delay

# Reset
curl -X POST http://localhost:8090/v1/gateway/scenarios \
  -H "Authorization: Basic $CREDS" \
  -d '{"mode":"success"}'
```

### 5. Outbox relay crash and recovery
```bash
# The outbox relay runs as a goroutine in the API gateway.
# Stop the server:
kill $API_GATEWAY_PID

# Events accumulate in the outbox table:
psql $DATABASE_URL -c "SELECT COUNT(*) FROM public.outbox WHERE published_at IS NULL"

# Restart server — relay catches up automatically on startup
go run ./cmd/api-gateway

# Verify events published:
psql $DATABASE_URL -c "SELECT COUNT(*) FROM public.outbox WHERE published_at IS NULL"
```

## Expected Results

| Scenario | Expected behavior |
|----------|------------------|
| DB latency 5s | Requests timeout, 503 returned, retry succeeds when latency removed |
| DB down | 503 on all writes, 200 on cached reads (if any) |
| Redis down | Idempotency falls back to DB, no duplicate payments |
| Webhook endpoint down | Webhook retried with exponential backoff, dead-lettered after 18 attempts |
| Outbox relay crash | Events queue in DB, published in order on restart |

## Automated Chaos Test

See `tests/chaos/chaos_test.go` for a Go-based automated chaos test that:
1. Starts Toxiproxy programmatically
2. Injects failures during payment capture
3. Verifies idempotency prevents double charges
4. Verifies the DB remains consistent

Export either `CHAOS_AUTH_HEADER` or `CHAOS_API_KEY_ID` plus `CHAOS_API_KEY_SECRET`
before running the automated suite so the chaos client can authenticate.
