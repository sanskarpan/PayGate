# PayGate Performance Baseline

Measured 2026-08-11 against commit `bf61c16`. Every number here came from a command that is reproduced below. Nothing is estimated from reading code.

---

## Environment and how far to trust these numbers

| | |
|---|---|
| Host | Apple Silicon, 11 cores, macOS |
| Stack | api-gateway (native binary), Postgres 16 / Redis 7 / Kafka 3.8 (KRaft) in Docker |
| Database | dedicated `paygate_perf`, migrated from empty |
| Gateway | simulator pinned to `GATEWAY_SIM_SUCCESS_RATE=1` for determinism |
| Load generator | k6, same host as the API |
| Host load average during runs | **4.0–5.4**, from unrelated containers and processes |

> **Treat the absolute figures as indicative, not as a benchmark.** The host was not quiescent: a Docker VM and several unrelated workloads were consuming CPU throughout (`load averages: 4.73 4.09 5.42`), and k6 shares CPU with the API. Attempting to quiesce would have meant killing the owner's unrelated work.
>
> What *is* safe to rely on:
> * **Relative comparisons within a run** (authenticated vs unauthenticated on the same server, one endpoint vs another).
> * **Root causes**, which were confirmed independently of the load test — by direct microbenchmark, by reading the driver's default, and by inspecting live database session state.
> * **Order of magnitude.** The headline findings are 20×–70× effects, far larger than the noise floor.

---

## 1. Latency by endpoint

Closed-loop flow (order → authorize → capture → get → list → refund) at 8 flows/sec for 60s, deliberately below the rate limiter so this measures service time rather than queuing. 480 flows, 2,880 requests, **0 errors**.

### Client-side (k6, includes loopback network)

| Endpoint | p50 | p90 | p95 | max |
|---|---|---|---|---|
| `POST /v1/orders` | 85.3 ms | 214.0 ms | 324.1 ms | 780 ms |
| `POST /v1/payments/authorize` | 170.1 ms | 411.8 ms | 529.0 ms | 1.47 s |
| `POST /v1/payments/{id}/capture` | 88.3 ms | 200.1 ms | 252.1 ms | 884 ms |
| `POST /v1/payments/{id}/refunds` | 90.3 ms | 198.3 ms | 261.9 ms | 781 ms |
| `GET /v1/payments/{id}` | 72.4 ms | 127.0 ms | 160.4 ms | 530 ms |
| `GET /v1/payments` | 74.0 ms | 137.4 ms | 179.8 ms | 367 ms |

```bash
k6 run --env BASE_URL=http://127.0.0.1:18090 --env KEYS_FILE=perf_keys.json \
       --env RATE=8 --env DURATION=60s perf_latency.js
```

### Server-side (from the API's own `duration_ms`, excludes network)

Aggregated over ~1,300 requests per endpoint, paths normalised.

| Endpoint | N | p50 | p90 | p99 | max |
|---|---|---|---|---|---|
| `POST /v1/payments/authorize` | 1308 | 165 ms | 541 ms | 3,195 ms | 5,696 ms |
| `POST /v1/payments/{id}/refunds` | 1308 | 88 ms | 281 ms | 1,175 ms | 3,503 ms |
| `POST /v1/payments/{id}/capture` | 1308 | 84 ms | 283 ms | 975 ms | 3,353 ms |
| `POST /v1/orders` | 1368 | 78 ms | 334 ms | 1,804 ms | 2,974 ms |
| `GET /v1/payments` | 1308 | 71 ms | 191 ms | 630 ms | 2,876 ms |
| `GET /v1/payments/{id}` | 1308 | 69 ms | 185 ms | 589 ms | 1,455 ms |
| `POST /v1/card-tokens` | 20 | 61 ms | 71 ms | 75 ms | 75 ms |
| **`POST /v1/merchants`** *(unauthenticated)* | 21 | **3 ms** | 14 ms | 23 ms | 23 ms |

### Where the time actually goes

The last row is the whole story. `POST /v1/merchants` writes a row to the same database on the same server and takes **3 ms**. Every *authenticated* endpoint has a floor around **57–70 ms**, including `GET /v1/payments/{id}`, which is a single indexed lookup.

The difference is the authentication middleware. Three independent confirmations:

**(a) Direct microbenchmark of the hash comparison**

```
bcrypt cost in use: 10
CompareHashAndPassword: 55.62ms per call -> theoretical max 18.0 auths/sec/core
  cost= 4 ->   0.87ms/verify
  cost= 6 ->   3.41ms/verify
  cost= 8 ->  13.57ms/verify
  cost=10 ->  56.06ms/verify
```

`internal/merchant/service.go:193` runs `bcrypt.CompareHashAndPassword` at `bcrypt.DefaultCost` on **every authenticated request**, and then issues an additional `UpdateAPIKeyLastUsed` write.

**(b) The 55.6 ms accounts for the entire observed floor.** 55.6 ms of bcrypt + ~3 ms of query ≈ the 57 ms minimum and 69 ms p50 measured on the cheapest authenticated read.

**(c) CPU under load** — API process CPU serving only `GET /v1/payments`:

```
idle CPU:            0.7%
under load CPU:    195.3%   (~2 cores)
under load CPU:    138.7%
under load CPU:    197.1%
```

Note what bcrypt is *for*: slowing offline brute-force of low-entropy, human-chosen passwords. PayGate's API secrets are **32 bytes from `crypto/rand`** (`generateSecret`, `internal/merchant/service.go:365`) — 256 bits of entropy. Key-stretching contributes nothing against a secret that cannot be guessed, while costing 55.6 ms and roughly 18 verifications per second per core.

---

## 2. Throughput and saturation

Ramp on a cheap authenticated read, `GET /v1/payments?count=10`, 20 → 300 req/s over 100s. **60 distinct API keys** were used so the per-key rate limiter had 600 rps of headroom and could not be the binding constraint.

```
lat_read..............: min=56.72ms med=3.31s p(90)=5.3s p(99)=6.53s p(99.9)=6.72s max=6.85s
http_reqs.............: 7711   75.54/s
dropped_iterations....: 5488   53.76/s
rate_limited..........: 0.00%  0 out of 7711
failed (5xx)..........: 0.00%  0 out of 7711
```

**Saturation point: ~75 req/s.** Beyond it the arrival rate could not be sustained — k6 dropped 5,488 iterations because every VU was blocked — and latency degraded from a 57 ms floor to a **3.31 s median**.

**What saturates first: CPU in the authentication middleware.** Not the connection pool, not lock contention, not Kafka. The evidence: zero requests were rate-limited, zero 5xx, and 75 req/s × 55.6 ms ≈ **4.2 CPU-cores' worth of bcrypt per wall-clock second** on a host already ~40% busy with unrelated work.

```bash
k6 run --summary-trend-stats="min,med,p(90),p(99),p(99.9),max,avg" \
       --env KEYS_FILE=perf_keys_all.json perf_saturation.js
```

### Rate limiting, separately

`middleware.NewRateLimiter(10, 20)` — 10 rps sustained, burst 20, keyed on **(API key, request path)**, held in an in-memory map **per process**. Consequences:

* A single merchant is capped at 10 rps *per endpoint*, which is why the load harness needed 60 merchants to reach 75 rps.
* The limit does not hold across replicas: N replicas allow 10N rps. Redis is already a dependency and is not used for this.
* `internal/ratelimit/tier.go` contains a complete tier-based limiter (free/standard/enterprise at 10/100/1000 rps) that is **referenced nowhere**.
* **`/healthz` is rate limited too.** With no `Authorization` header the key falls back to client IP, so a load balancer health-checking from one IP faster than 10 rps receives `429`. Measured: a ramp against `/healthz` returned **92% non-2xx**.

---

## 3. Async pipeline — the largest gap

### Outbox relay throughput: **1.00 events/sec**

```bash
# backlog delta over a 300-second window, no API traffic during the window
backlog 8238 -> 7938 over 300s  => drain 1.00 events/sec
batches published in window: 4
```

During the latency run the system *produced* roughly 77 events/sec (9,216 outbox rows in about two minutes). It drains at **1 event/sec**. The backlog is therefore unbounded under any sustained load, and it never recovered: 8,538 events were still unpublished long after traffic stopped.

Observed publish lag on the events that did get through:

| events | p50 | p90 | p99 | max |
|---|---|---|---|---|
| 678 | **223.9 s** | 576.6 s | 578.1 s | 578.2 s |

**Root cause — confirmed, and it matches the measurement to two decimal places.**

`internal/outbox/kafka.go` builds a `kafka.Writer` with `Async: false` and **does not set `BatchTimeout`**. In kafka-go v0.4.51:

```go
func (w *Writer) batchTimeout() time.Duration {
    if w.BatchTimeout > 0 { return w.BatchTimeout }
    return 1 * time.Second        // writer.go:825-830
}
```

Each `WriteMessages` call carries a single message and blocks until the batch timer fires. One second per event. Measured drain: **1.00 events/sec**.

Two aggravating factors in `Relay.PublishBatch` (`internal/outbox/relay.go:127`):

1. All 100 publishes happen **inside an open database transaction**, which is only committed after the last one. Live session state during a run:
   ```
   pid 67711 | idle in transaction | 00:01:27 | UPDATE "public"."outbox" SET published_at = NOW() WHERE id = ...
   ```
   A transaction held open for 87 seconds also blocks vacuum and retains row locks for the whole batch.
2. Any single publish failure returns an error for the **entire batch**, rolling back all of it — head-of-line blocking, where one bad event stalls every event behind it.

### Kafka consumer

Not separately measurable in this run: the producer never got far enough ahead to build meaningful consumer lag. The consumer-group configuration itself was fixed earlier (single reader with `GroupTopics`); see `issues.md`.

---

## 4. Database

Hot paths are correctly indexed. Scan counts after the load run:

| table | live rows | size | seq scans | idx scans |
|---|---|---|---|---|
| `outbox` | 9,216 | 3.5 MB | 49 | 563 |
| `idempotency_records` | 5,292 | 7.6 MB | 206 | 15,674 |
| `payments` | 1,406 | 2.7 MB | 9 | 49,066 |
| `orders` | 1,368 | 1.3 MB | 220 | 3,724 |

**The `FOR UPDATE` order lock added by the overcharge fix is not a bottleneck.** The exposure query it introduced is fully index-covered:

```
Aggregate (actual time=10.248..10.249 rows=1 loops=1)
  Buffers: shared hit=3
  ->  Index Scan using idx_payments_order on payments
        Index Cond: (order_id = 'order_...')
        Filter: (merchant_id = ... AND status <> ALL (...))
        Buffers: shared hit=3
```

Three buffer hits, no sequential scan. Lock contention only arises for concurrent authorizations **against the same order**, which is not a normal access pattern.

`idempotency_records` is worth watching: it is the largest table by bytes (7.6 MB for 5,292 rows, because it stores response bodies) and took 206 sequential scans.

### Limitation

`pg_stat_statements` is **not enabled** (`shared_preload_libraries` is empty), so per-statement timings could not be captured without restarting Postgres and re-running the load. `EXPLAIN ANALYZE` on specific queries was used instead. Enabling it is a prerequisite for the next round.

---

## 5. Frontend

**Not measured in this pass.** Core Web Vitals, bundle sizes and per-route server-render times remain open. Given that every dashboard page is server-rendered and every server render performs authenticated API calls that each pay the 55.6 ms bcrypt cost, frontend numbers taken before fixing §1 would mostly re-measure the same bottleneck. Sequenced after it.

---

## Reproducing this

```bash
# 1. clean database
psql -h localhost -p 5435 -U paygate -d postgres -c "CREATE DATABASE paygate_perf OWNER paygate;"
migrate -path ./migrations -database "postgres://…/paygate_perf?sslmode=disable" up

# 2. API with a deterministic gateway
PORT=18090 DATABASE_URL=… REDIS_ADDR=localhost:6380 KAFKA_BROKERS=localhost:9092 \
  GATEWAY_SIM_SUCCESS_RATE=1 ./paygate-api

# 3. seed N merchants, each with a key and a reusable card token
N=60 bash mk_keys.sh

# 4. latency, then saturation
k6 run --env RATE=8 --env DURATION=60s perf_latency.js
k6 run --summary-trend-stats="min,med,p(90),p(99),p(99.9),max,avg" perf_saturation.js

# 5. server-side percentiles from the API log
#    (group by normalised path over "duration_ms")

# 6. outbox drain rate
psql -tAc "select count(*) from public.outbox where published_at is null"   # wait 300s, repeat
```

---

## Headline numbers

| Metric | Measured | Cause |
|---|---|---|
| Fixed cost per authenticated request | **~56 ms** | bcrypt cost 10 on every request |
| Authenticated request ceiling | **~18/sec/core** | same |
| Observed saturation | **~75 req/s** | CPU in auth middleware |
| Latency at saturation | **3.31 s median**, 6.53 s p99 | queuing behind saturated CPU |
| Outbox drain rate | **1.00 events/sec** | kafka-go default 1 s `BatchTimeout` |
| Outbox publish lag | **p50 224 s, p99 578 s** | same, plus batch-in-transaction |
| Per-merchant API rate limit | **10 rps per endpoint**, per process | in-memory limiter, not shared |
