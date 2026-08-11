# PayGate — Gap Analysis and Roadmap

Compiled 2026-08-11 against commit `bf61c16`. Inputs: measured performance (`PERFORMANCE-BASELINE.md`), documented competitor capabilities (`COMPETITIVE-ANALYSIS.md`), and the resolved audit register (`issues.md`).

Evidence labels used throughout:
* **[M]** measured — a command was run and the number is reproducible
* **[D]** documented — a competitor's public source, cited in `COMPETITIVE-ANALYSIS.md`
* **[I]** inferred — engineering judgment, explicitly flagged as such

Cost: **S** ≤ 1 day · **M** ≤ 1 week · **L** > 1 week

---

## Ranked by impact

Ordered by consequence, not by ease. A correctness or compliance gap outranks any latency win.

---

### GAP-01 — Merchant signup is unauthenticated · P0 · S

**What's missing.** `POST /v1/merchants` requires no credentials, and each new merchant may mint one admin API key with no credentials **[M]** — verified live during the audit. Anyone who can reach the API can create unlimited tenants and keys.

**Who has it.** All seven comparators gate merchant creation **[D]**.

**Impact.** Unbounded anonymous tenant and key creation; a resource-exhaustion and abuse vector, and disqualifying for any real deployment.

**Dependencies.** None.

**Cost.** S — a platform-level credential or invite token on the two bootstrap endpoints. The per-merchant guards after the first key are already correct **[M]**.

---

### GAP-02 — bcrypt on every authenticated request caps throughput at ~18 req/s/core · P0 · S

**What's missing.** `AuthenticateAPIKey` runs `bcrypt.CompareHashAndPassword` at cost 10 on **every** request **[M]**: 55.6 ms per verification, ~18 verifications/sec/core. It also issues an extra `UpdateAPIKeyLastUsed` write per request.

**Evidence.** Measured saturation **~75 req/s** with latency collapsing from a 57 ms floor to a **3.31 s median**, zero rate-limiting and zero 5xx, with the API burning ~195% CPU on a trivial authenticated read **[M]**. Unauthenticated `POST /v1/merchants` on the same server: **3 ms p50** **[M]**.

**Who has it.** No competitor publishes its auth implementation, but Stripe sustains a documented 100 rps *per account* **[D]**, which is not reachable at 18 verifications/sec/core **[I]**.

**Impact.** This is simultaneously the entire latency floor and the throughput ceiling. Every other performance number in the baseline is downstream of it, including dashboard page loads, which fan out to several authenticated calls per render.

**Why it is safe to change.** API secrets are 32 bytes from `crypto/rand` — 256 bits **[M]**. Key-stretching exists to slow offline brute-force of low-entropy human passwords; it contributes nothing against a secret that cannot be guessed. Replace with HMAC-SHA256 (or SHA-256) plus constant-time comparison — microseconds instead of 55.6 ms.

**Keep bcrypt** for dashboard *user passwords* (`service.go:304/320`), which are human-chosen and genuinely need stretching.

**Dependencies.** Requires a hash-scheme migration: store the new digest alongside the existing bcrypt hash, verify against either, backfill on next use, then drop the bcrypt column. Existing keys must keep working.

**Cost.** S for the code; M including safe migration.

---

### GAP-03 — Outbox relay drains at 1 event/sec · P0 · S

**What's missing.** The relay publishes **1.00 events/sec** against ~77 events/sec produced under load **[M]** — an unbounded backlog. Measured publish lag **p50 224 s, p99 578 s**; 8,538 events were still unpublished long after traffic stopped.

**Root cause, confirmed to two decimal places.** `internal/outbox/kafka.go` sets `Async: false` and leaves `BatchTimeout` unset; kafka-go v0.4.51 defaults it to **1 second** (`writer.go:825-830`), and each `WriteMessages` carries a single message **[M]**.

**Two aggravating factors [M].** All 100 publishes run **inside an open database transaction** — an `idle in transaction` session was observed at 87 seconds, which also blocks vacuum. And any single publish failure rolls back the whole batch: head-of-line blocking where one bad event stalls everything behind it.

**Who has it.** Adyen retries webhooks for up to 30 days, Hyperswitch 16 times over 24 h, Checkout.com 8 times over ~30 h **[D]**. None of that is achievable when the *producer* moves 1 event/sec.

**Impact.** Webhooks are effectively ~10 minutes late under trivial load and unbounded under real load. For a payments platform this is the most severe operational gap in the system.

**Fix.** Set `BatchTimeout` (10 ms), pass the whole batch to one `WriteMessages` call (kafka-go is variadic), and move publishing outside the database transaction — mark rows published in a second short transaction after the broker acks. Isolate per-event failures so one poison message cannot stall the queue.

**Dependencies.** None. Independent of GAP-02.

**Cost.** S for `BatchTimeout` alone (likely a 100× improvement); M for the transaction restructuring and per-event failure isolation.

---

### GAP-04 — No SCA / 3DS2 exemption handling · P1 · L

**What's missing.** PayGate simulates a 3DS challenge but has no exemption concept at all.

**Who has it.** Adyen exposes a four-value enum (`lowValue`, `secureCorporate`, `trustedBeneficiary`, `transactionRiskAnalysis`); Checkout.com a six-value `challenge_indicator`; Stripe requests exemptions automatically (TRA, low-value, MIT, data-only) **[D]**.

**Impact.** Directly reduces authorization rates and changes liability shift in Europe. Higher business impact than any latency work here **[I]** — an unnecessary challenge costs a conversion, and every competitor optimises this.

**Dependencies.** Requires real 3DS integration, so it follows real acquiring rails.

**Cost.** L.

---

### GAP-05 — Rate limiting does not survive more than one replica · P1 · M

**What's missing.** `middleware.NewRateLimiter(10, 20)` — 10 rps, burst 20, keyed on (API key, path), in an **in-memory map per process** **[M]**. N replicas therefore allow 10N rps. A complete tier-based limiter exists at `internal/ratelimit/tier.go` (free/standard/enterprise at 10/100/1000 rps) and is **referenced nowhere** **[M]**. Redis is already a dependency and is unused for this.

**Also:** `/healthz` is rate limited. With no `Authorization` header the key falls back to client IP, so a load balancer health-checking from one IP above 10 rps gets `429` — measured **92% non-2xx** on a ramp **[M]**.

**Who has it.** Stripe 100 rps live / 25 sandbox, Checkout.com 100 rps read + 100 write, Cashfree per-endpoint, Hyperswitch 80 rps — all published **[D]**. Razorpay and PayU publish none **[D]**.

**Impact.** The published limit is meaningless under horizontal scaling, and 10 rps per endpoint is far below every published competitor limit. Health checks break at scale.

**Cost.** M — wire the existing tiered limiter to Redis, exempt `/healthz` and `/readyz`, publish the numbers.

---

### GAP-06 — No published operational contract · P1 · S

**What's missing.** No documented rate limits, webhook retry schedule, delivery guarantee, idempotency TTL, API versioning policy, SLA, or status page.

**The gap is documentation, not behaviour.** The underlying implementation is often already competitive **[M]**: idempotency returns `409` on key reuse with a different body and 8 parallel identical requests create exactly one order; webhooks are HMAC-signed with RFC 9421 structured signatures; delivery is at-least-once with a dead-letter path.

**Who has it.** Adyen publishes an exact retry ladder (9s, 18s, 27s, then 2m…8h, up to 30 days) and URL-pinned API versions with a diff tool; Hyperswitch publishes 16 retries over 24 h plus explicit at-least-once/unordered/dedupe semantics; Checkout.com publishes a 24 h idempotency TTL **[D]**. PayU publishes four inconsistent webhook contracts and no guarantee at all **[D]**.

**Impact.** Integrators cannot build correct retry, dedupe or backoff logic against undocumented semantics. This is the cheapest credibility win available.

**Cost.** S — write down what the code already does, then keep it honest with tests.

---

### GAP-07 — API versioning has no mechanism · P1 · M

**What's missing.** A `/v1` path prefix and no version negotiation. Any breaking change breaks every client simultaneously.

**Who has it.** Stripe uses a dated `Stripe-Version` header with named release trains; Adyen pins versions in the URL and ships an API diff tool; Cashfree uses a dated `x-api-version` **[D]**. Checkout.com and Razorpay have only compatibility policies **[D]**.

**Impact.** Blocks evolving the API safely once real integrators exist. Adyen's URL-pinned model is the safest to copy **[I]**.

**Cost.** M.

---

### GAP-08 — Dashboard write coverage is incomplete · P2 · L

**What's missing.** Webhooks, settlements, recon, mandates, and capture/refund are **read-only in the UI despite the backend exposing writes** **[M]** (`issues.md`).

**Who has it.** All seven comparators offer full dashboard operations **[D]**.

**Impact.** Operators must fall back to API calls for routine work, which undercuts the operator-console positioning.

**Cost.** L — six page groups; each is small, the aggregate is not.

---

### GAP-09 — No caching layer · P2 · M

**What's missing.** Every dashboard page load performs synchronous authenticated API calls; nothing is cached anywhere.

**Impact.** Amplifies GAP-02: a single page render costing several authenticated calls pays ~56 ms of bcrypt per call **[M]**.

**Sequencing note.** Fix GAP-02 first. A large part of the apparent need for caching is bcrypt, and caching in front of an artificially slow auth path would hide the real defect **[I]**.

**Cost.** M.

---

### GAP-10 — Payment-method coverage · P2 · L

**What's missing.** No EMI, BNPL, international cards, network tokenization, Apple/Google Pay; partial capture and multi-capture absent.

**Who has it.** Partial capture: Stripe, Adyen, Checkout.com, Cashfree **[D]** — Razorpay notably also lacks it, and PayU allows only one **[D]**. Multi-capture: Checkout.com (`NonFinal`, 150-action cap) and Hyperswitch (`manual_multiple`) are the cleanest models **[D]**.

**Impact.** Table stakes for Indian market parity; bounded by having no real acquiring rails **[I]**.

**Cost.** L.

---

### GAP-11 — Smaller correctness and hygiene items · P3 · S each

* **`idempotency_records` growth** — largest table by bytes (7.6 MB for 5,292 rows, storing response bodies), 206 sequential scans **[M]**. Needs a retention policy and an index review.
* **`pg_stat_statements` not enabled** **[M]** — blocks per-statement analysis; prerequisite for the next performance round.
* **Dashboard build is not hermetic** — requires egress to Google Fonts.
* **Integration tests not fully re-runnable** — hardcoded event IDs.
* **gosec annotations** — 5 pre-existing false positives will fail `lint` on a golangci-lint bump **[M]**.
* **Only 2 server SDKs** (Go, JS) versus 6–7 for most competitors **[D]**.

---

## Roadmap

### Track 1 — Table-stakes parity gaps

Correctness and compliance. Nothing else should precede these.

| Order | Gap | Cost | Rationale |
|---|---|---|---|
| 1 | GAP-01 signup authentication | S | Open door; hours to close |
| 2 | GAP-06 publish the operational contract | S | Documents behaviour that already works |
| 3 | GAP-05 distributed, tiered rate limiting | M | Current limiter is per-process; the replacement already exists unused |
| 4 | GAP-07 API versioning | M | Must land before external integrators, not after |
| 5 | GAP-04 SCA exemptions | L | Highest revenue impact; gated on real rails |
| 6 | GAP-10 method coverage, partial/multi-capture | L | Market parity |
| 7 | GAP-08 dashboard write coverage | L | Product completeness |

### Track 2 — Performance work, with measured justification

Every item below is justified by a number in `PERFORMANCE-BASELINE.md`. Nothing is here on suspicion.

| Order | Work | Measured justification | Expected effect | Cost |
|---|---|---|---|---|
| 1 | **Outbox `BatchTimeout`** (GAP-03) | 1.00 events/sec drain, exactly matching kafka-go's 1 s default | ~100× on the async pipeline for a one-line change **[I]** | S |
| 2 | **Replace bcrypt in the API-key path** (GAP-02) | 55.6 ms/request; saturation ~75 req/s; 195% CPU on a trivial read | Removes the latency floor and the throughput ceiling together | S + migration |
| 3 | Publish outbox outside the DB transaction | 87 s `idle in transaction` observed | Stops vacuum blocking and head-of-line stalls | M |
| 4 | Exempt `/healthz` and `/readyz` from rate limiting | 92% non-2xx on a health-check ramp | Health checks work behind a load balancer | S |
| 5 | Drop the extra `UpdateAPIKeyLastUsed` write per request | one write per authenticated request | Removes a write from every request path | S |
| 6 | Enable `pg_stat_statements`, re-measure | currently unavailable | Prerequisite for the next round | S |
| 7 | Re-measure on a quiesced host; add frontend CWV | host load 4.0–5.4 during runs | Turns indicative numbers into a real benchmark | M |

**Explicitly not scheduled yet:** read replicas, partitioning, connection-pool tuning, CDN/edge, payload compression. Nothing measured points at them. The database is comfortably indexed, the `FOR UPDATE` order lock is index-covered with three buffer hits, and CPU in the auth path saturates long before any of these become the constraint **[M]**. Revisit after items 1–2, when the real ceiling is visible.

### Track 3 — Differentiators

Where PayGate can be better rather than equal. All verified, not aspirational.

| Differentiator | Evidence | Move |
|---|---|---|
| **Double-entry ledger as a product surface** | Balanced to zero across every transaction under live traffic **[M]**; no competitor exposes ledger primitives **[D]** | Make it a documented, queryable API — the thing merchants reconcile against |
| **Published latency percentiles** | Nobody in this field publishes them **[D]**; PayGate already measures them **[M]** | Publish p50/p99 with methodology once Track 2 items 1–2 land |
| **Published uptime SLA + public status page** | Only Adyen publishes a contractual figure; Checkout.com's status page is login-gated and PayU's returns 404 **[D]** | A real number plus an open status page beats most of the field |
| **Verified isolation and idempotency** | Cross-tenant isolation and single-order-under-8-parallel-requests both adversarially verified **[M]** | Publish the test methodology, not just the claim |
| **Merchant-configurable velocity rules** | Stripe, Adyen and Checkout.com only offer this partially **[D]** | Already ahead; document it |

---

## What this analysis does not cover

Stated rather than quietly omitted:

* **Frontend performance** — no Core Web Vitals, bundle sizes or per-route render times. Deferred deliberately: every server render fans out to authenticated calls that each pay 56 ms of bcrypt, so measuring before GAP-02 would mostly re-measure that **[I]**.
* **Absolute performance comparison** — the host was not quiescent (load 4.0–5.4). Figures are reliable as relative comparisons and root causes, not as benchmark numbers.
* **Per-statement database analysis** — `pg_stat_statements` was not enabled.
* **Kafka consumer lag under sustained load** — the producer never got far enough ahead to build meaningful consumer lag; blocked on GAP-03.
* **Competitor pricing, contractual terms and negotiated limits** — public documentation only.
* **`FOR UPDATE` lock contention under same-order concurrency** — the query is index-covered with three buffer hits **[M]**, but concurrent authorization of one order was not load-tested. Not a normal access pattern **[I]**.
