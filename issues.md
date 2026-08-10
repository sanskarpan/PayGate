# PayGate QA Audit — Issue Register

Audit date: 2026-08-09 · Base commit: `712db8e` · Method: live stack (API + dashboard + Postgres + Redis + Kafka), real browser automation, direct API probing, server-log and database inspection.

Environment: dedicated `paygate_qa` database, Kafka topics wiped, two seeded merchants — one fully populated, one empty (for empty-state coverage).

---

## Summary

| Severity | Found | Resolved | Open |
|---|---|---|---|
| P0 / Critical | 1 | 1 | 0 |
| P1 / High | 5 | 5 | 0 |
| P2 / Medium | 4 | 3 | 1 |
| P3 / Low | 3 | 1 | 2 |
| **Total** | **13** | **10** | **3** |

Open items are all deliberate deferrals with a stated reason; none are silently skipped.

---

## ISSUE-001 — An order can be charged an unlimited multiple of its amount

- **Severity:** Critical · **Priority:** P0 · **Type:** Functional / Data integrity / Money loss
- **Area:** Payments · **Route:** `POST /v1/payments/authorize` → `POST /v1/payments/{id}/capture`
- **Status:** Resolved

### Description
An order could be authorized any number of times for its full amount, and every one of those authorizations could then be captured. A ₹500 order was charged ₹2,000.

### Steps to Reproduce
1. Create an order for `50000` with `partial_payment: false`.
2. Call `/v1/payments/authorize` for `50000` **four times** (all succeed — the order is still `attempted`).
3. Capture each of the four payments.

### Expected Behavior
Total live exposure across an order's payments never exceeds the order amount.

### Actual Behavior
Four payments captured. Verified in the database:
```
order amount = 50000
payments: 4 captured, total = 200000
ledger:   CUSTOMER_RECEIVABLE dr 200000 | MERCHANT_PAYABLE cr 196000 | PLATFORM_FEE_REVENUE cr 4000
order row still reports amount_paid = 50000, amount_due = 0, status = paid
```
Real money moved at 4× the order value, and the ledger disagreed with the order by 150000.

### Root Cause
`internal/payment/postgres.go` `StartAuthorization` locks the order `FOR UPDATE` but only rejects when the order status is already `paid`, and only validates the *single incoming* amount. The status stays `attempted` between authorize and capture, leaving an unbounded window for additional full-amount authorizations.

### Dependencies
ISSUE-002 masked how often this is reachable in practice: without an `Idempotency-Key`, the second authorize used to 500 before it could create a second payment.

### Fix
Inside the same locked transaction, sum the amounts of all non-terminal payments for the order and reject when `existing + incoming > order.amount`. Payments in `failed`, `authorization_reversed` and `auto_refunded` release their claim. Returns `409 ORDER_AMOUNT_EXCEEDED`.

### Verification
Repeating the exact reproduction: attempt 1 → `201`, attempts 2–4 → `409`; database shows **1 payment, total 50000**.

### Regression Tested
Yes — single-payment flow (authorize → capture → refund) unchanged; partial-payment orders still accept multiple payments and correctly reject the one that would exceed the total (40000 + 40000 accepted, third rejected against a 100000 order).

---

## ISSUE-002 — Second authorize without an `Idempotency-Key` returns 500

- **Severity:** High · **Priority:** P1 · **Type:** API / Runtime
- **Route:** `POST /v1/payments/authorize` · **Status:** Resolved

### Description
Authorizing twice on the same order without supplying an `Idempotency-Key` header returned `500`, making a legitimate retry impossible unless the caller supplied distinct keys.

### Root Cause
`StartAuthorization` inserted `in.IdempotencyKey` raw, so an absent header was stored as `''` rather than `NULL`. The partial unique index `idx_attempts_idempotency ... WHERE idempotency_key IS NOT NULL` therefore covered those rows and the second insert collided. The recovery branch is guarded by `in.IdempotencyKey != ""`, so it was skipped and the raw driver error fell through to a 500.

### Fix
Wrap the value in the existing `nullableText` helper, matching the neighbouring nullable columns.

### Verification
Two consecutive authorizes with no key on a partial-payment order → `201`, `201`.

---

## ISSUE-003 — Capability Enable/Restrict is completely non-functional (405)

- **Severity:** High · **Priority:** P1 · **Type:** Functional / API
- **Route:** `/compliance` → `PUT /v1/merchants/me/capabilities` · **Status:** Resolved

### Description
Every capability control on the compliance console failed. No capability could be enabled or restricted from the dashboard.

### Root Cause
`dashboard/components/compliance-ops-console.tsx` — the shared `run()` helper hardcoded `method: "POST"`, but the route is registered as `PUT` (`internal/merchant/handler.go:64`). The other three `run()` callers are genuinely POST endpoints, so only the capability path was affected.

### Fix
`run()` takes an HTTP method (default `POST`); `setCapability` passes `PUT`.

### Verification
Through the browser: RESTRICT → `200 PUT`, "Capability cards updated.", state survived a reload; ENABLE → `200 PUT`. `GET /v1/merchants/me/capabilities` confirms the round trip.

---

## ISSUE-004 — `screenings/run` returns 500, and the dashboard always sent an invalid type

- **Severity:** High · **Priority:** P1 · **Type:** Functional / Backend / Data integrity
- **Route:** `POST /v1/merchants/me/onboarding/screenings/run` · **Status:** Resolved

### Description
Two defects compounded: the dashboard sent `screening_type: "merchant_kyb"`, which is not an accepted value, and the backend answered `500` instead of rejecting it. The control never worked.

### Root Cause
`migrations/000033` constrains the column to `('kyb','beneficial_owner','controller')`. The service inserted the caller's value unvalidated, so the constraint violation surfaced as an unhandled 500.

### Fix
Validate against the accepted set in the service (`ErrInvalidScreeningType` → 400); correct the dashboard to send `kyb`.

### Verification
`merchant_kyb` → `400` naming the accepted values; `kyb` → `201`; the dashboard button now produces screening rows (3 present, all `kyb`).

---

## ISSUE-005 — `PUT /v1/risk/config` returns 500 and silently zeroes omitted fields

- **Severity:** High · **Priority:** P1 · **Type:** API / Data integrity
- **Status:** Resolved

### Description
Any request that was not fully populated and fully valid returned `500`. Because every field was overwritten unconditionally, a partial update would also wipe unspecified thresholds to zero — partial updates were impossible by construction.

### Root Cause
`internal/risk/handler.go` `upsertFraudConfig` seeded from defaults then assigned **every** field from the decoded body. Go zero-values omitted JSON fields, so the write violated `CHECK (ip_velocity_threshold > 0)` and friends.

### Fix
Decode into pointer fields, seed from the stored config, apply only supplied fields, and validate ranges up front (400 naming the offending field).

### Verification
`{}` → `200` no-op with values intact; `{"review_threshold":-5}` → `400 "review_threshold must not be negative"`; `{"review_threshold":25}` changed only that field (block/ip/spike unchanged).

---

## ISSUE-006 — Unrecognized payment `method` returns 500

- **Severity:** High · **Priority:** P1 · **Type:** API / Validation
- **Status:** Resolved

`"CARD"`, `"telepathy"` and `"<script>alert(1)</script>"` all returned `500` — the `payment_attempts_method_check` constraint rejected them with no app-level validation. Note `method` is not case-normalized even though `currency` is.

**Fix:** validate the enum in `StartAuthorization` (`ErrInvalidPaymentMethod` → 400).
**Verification:** `"CARD"` → `400 "method must be one of card, upi, netbanking, wallet"`.

---

## ISSUE-007 — NUL byte in any text field returns 500, including on an unauthenticated endpoint

- **Severity:** Medium · **Priority:** P2 · **Type:** Runtime / Availability
- **Status:** Resolved

A `\u0000` in `receipt`, `notes` keys or values, `POST /v1/merchants` `name` (**no auth required**), or a path parameter produced `500` (`invalid byte sequence for encoding "UTF8": 0x00`). Unauthenticated 500 generation is an error-budget and availability concern.

**Fix:** a `RejectNullBytes` middleware refusing a raw `0x00` or a `\u0000` escape in the URL or body, returning 400. Bounded by the existing `MaxBody` limit; writes the error inline to avoid an import cycle with `httpx`.
**Verification:** body escape, unauthenticated merchant create, and `%00` path param all → `400`. Normal orders including unicode (`café ✓ 日本`) still `201`.

---

## ISSUE-008 — Duplicate card token returns 500

- **Severity:** Medium · **Priority:** P2 · **Type:** API · **Status:** Resolved

Re-saving the same card (same merchant, PAN, expiry, class) hit `idx_card_tokens_fingerprint_active` and returned `500`. This is the ordinary "returning customer re-saves their card" path.

**Fix:** map `23505` to `ErrCardTokenExists` → `409 CARD_TOKEN_EXISTS`.
**Verification:** duplicate → `409` with a clear message.

---

## ISSUE-009 — Invalid pagination cursor returns 500

- **Severity:** Medium · **Priority:** P2 · **Type:** API · **Status:** Resolved

Any non-empty invalid `cursor` on `/v1/orders` returned `500` — including a truncated copy of a genuine cursor, a realistic client bug.

**Fix:** `ErrInvalidCursor` → 400.
**Verification:** `?cursor=abc` → `400 "cursor is not a valid pagination cursor"`.

---

## ISSUE-010 — Refund create response returns a zero timestamp

- **Severity:** Low · **Priority:** P3 · **Type:** API · **Status:** Resolved

The `201` response carried `created_at: -62135596800` / `0001-01-01T00:00:00Z` while the stored row was correct. Clients persisting `created_at` from the create response stored a negative epoch.

**Root cause:** `ProcessRefund` returned a struct populated by a partial `SELECT` that omits `created_at`.
**Fix:** re-read the committed row via `GetRefund`.
**Verification:** `created_at: 1786267672`, `created_at_rfc: 2026-08-09T14:57:52+05:30`.

---

## ISSUE-011 — Gateway decline and timeout scenarios never produce a failed payment

- **Severity:** Medium · **Priority:** P2 · **Type:** Functional / Design decision
- **Route:** `/gateway` · **Status:** OPEN — needs a product decision, deliberately not changed

### Description
Setting the "Decline All" or "Timeout" scenario on the Failure-path command deck has no observable effect on card authorization: payments still come back `authorized`. `challenge` works correctly.

### Root Cause
`defaultRoutingPolicy` sets `FallbackProvider: simulator_failover` with `FailoverOnDecline: true` and `FailoverOnError: true`. When the scenario declines on the primary, the router fails over to `simulator_failover`, whose `AuthorizeWithProvider` returns success **unconditionally, ignoring the active scenario**. `challenge` is unaffected because `RequiresAction` is treated as terminal and short-circuits failover.

### Why this is left open
This behaviour is **intentional and covered by an existing integration test** (`tests/integration/open_issue_batch_two_integration_test.go`) which asserts that a declined primary fails over and that `provider == simulator_failover`. Changing it would break a deliberate, tested contract, so it is not a unilateral fix.

Two separable concerns for the owner to decide:
1. **Feature accuracy** — the failure-rehearsal surface cannot rehearse declines or timeouts, which is most of its stated purpose. Options: have `simulator_failover` honour the active scenario, or expose the routing policy on the `/gateway` page so an operator can disable failover while rehearsing.
2. **`FailoverOnDecline: true` as a default** — in real payments an issuer decline (insufficient funds, suspected fraud, stolen card) is a business decision, not a provider outage. Silently retrying it on a second provider is generally contrary to scheme rules and can look like a duplicate-authorization attempt. Worth reconsidering as a default even though it is configurable per policy.

---

## ISSUE-012 — API key creation returns `key_id` while the list returns `id`

- **Severity:** Low · **Priority:** P3 · **Type:** API consistency · **Status:** OPEN

`POST /v1/merchants/me/api-keys` returns `key_id`; `GET` returns `id` for the same field. A client reading `id` from the create response gets `undefined`.

Left open because it is a public contract change; the additive fix (include `id` alongside `key_id`) should be taken with a versioning decision.

---

## ISSUE-013 — No length cap on `receipt`/`notes`; webhook event names unvalidated

- **Severity:** Low · **Priority:** P3 · **Type:** Validation · **Status:** OPEN

A 100,000-character `receipt` is accepted and stored in full, and `POST /v1/webhooks {"events":["not.a.real.event"]}` returns `201`, creating a subscription that can never fire. Neither causes an error today; both are storage-growth and silent-misconfiguration risks. Left open as they need product-level limits rather than an arbitrary constant.

---

## Non-issue: Playwright `webServer` rebuilds `.next` under a running server

Recorded because it produced a convincing false positive during this audit — eight core routes appeared completely broken with `ChunkLoadError` and blank pages.

Running the Playwright suite executes `pnpm build`, replacing `.next` with new chunk hashes. A `next start` server already running against the old build then serves HTML referencing chunks that no longer exist. Compounded by `pkill -f 'next start'` not matching the real process name, which is `next-server`.

No production impact. After killing all `next-server` processes, deleting `.next`, rebuilding and starting a single server, all 100 route checks passed.

---

## Verified correct — no defect

Recorded so these are not re-investigated.

**Rendering.** 25 routes × (direct load + reload) × (desktop 1500px, mobile 390px) = **100/100 clean**: no console errors, no page errors, no failed requests, no horizontal overflow, no blank renders.

**Authentication.** 17/17 protected routes redirect unauthenticated users. Missing header, non-base64 Basic, Basic without colon, empty Basic, `Bearer`/garbage schemes, valid key with wrong secret, empty secret, nonexistent key, SQLi in key id, and key A's secret with key B's id all → 401. Revoked key → 401; double-revoke → 404; merchant B revoking A's key → 403.

**Authorization.** A read-scope key was refused on all 12 write operations tested, including `POST /v1/merchants/{id}/keys` (privilege escalation blocked). Read operations still succeed.

**Tenant isolation.** Merchant B using A's identifiers: GET order, GET payment, capture, refund, authorize-on-A's-order, GET/DELETE card token all → 404 (not 403, which correctly avoids resource enumeration). List endpoints never leak. In the browser, all six detail routes 404 with no identifier echoed.

**Session lifecycle.** Logout clears `paygate_dashboard_session`; re-access is blocked.

**Injection safety.** `'; DROP TABLE orders;--`, `1 OR 1=1; DELETE FROM ...`, `<script>alert(1)</script>`, `"><img src=x onerror=alert(1)>`, `{{7*7}}`/`${7*7}`/`<%= 7*7 %>` in `receipt` and in `notes` keys *and* values: stored verbatim, returned JSON-escaped, no 500, no template evaluation, tables intact.

**Amount validation.** Orders reject negative, zero, `0.5`, `1.7`, string, bool, array, object, null, missing, `1e30`, `1e400`, `maxint64+1`. Refunds reject over-refund, refund on an uncaptured payment, and all malformed amounts. Capture rejects over-authorized and malformed amounts.

**Idempotency.** Identical body replays return the same resource with `Idempotent-Replayed: true`; a different body with the same key → `409 IDEMPOTENCY_CONFLICT`; keys are scoped per merchant and per endpoint. **8 parallel identical requests created exactly 1 order**, confirmed twice.

**Concurrency.** 8 parallel 10000-refunds against a 50000 capture: exactly 5 succeeded, 3 correctly rejected, database sum exactly 50000. No overspend race.

**Malformed input.** Truncated JSON, trailing comma, unquoted key, empty body, `null`, `[]`, bare string/number, XML, form-urlencoded, 2000-deep nesting → all 400. `DELETE`/`PUT` on `/v1/orders` → 405.

**Invalid identifiers.** Nonexistent, wrong prefix, empty, whitespace, path traversal (encoded and raw), SQLi, XSS, 5000-character and unicode ids → 404 across orders, payments, card tokens, webhooks, settlements and disputes, with no crash.

**Pagination scalars.** `count`/`skip`/`limit` at 0, negative, 999999, non-numeric, maxint64 and fractional → 200, clamped, no unbounded result sets.

**Empty states.** All 10 list surfaces render empty copy with no page errors for a merchant with no data.

**Webhook SSRF.** `javascript:`, `file://`, `169.254.169.254`, non-URL and NUL-byte URLs → 400. Loopback over http is accepted by design in non-production and disabled in production.

**Working UI controls.** All 7 gateway scenario buttons (`201`), report export generation, tax-profile save, dispute START REVIEW, saga ABORT, beneficiary VERIFY and APPROVE.

**Controls that look inert but are correct.** Risk-queue and payout bulk actions require a selection; allowlist save/reset require a pending change; "Rotate credential" is disabled for a revoked key; "Save current view" uses `window.prompt`, which headless automation auto-dismisses; the team invite is blocked by native `required` validation. `409` on re-submitting dispute evidence and on replaying a non-replayable saga are the state machines working as designed.
