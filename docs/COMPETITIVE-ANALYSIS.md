# PayGate — Competitive Feature Matrix

Compiled 2026-08-11 against commit `bf61c16`. Competitor rows are **documented** — taken from each vendor's official documentation, with a source link. PayGate rows are **measured or read from this repository**. Where a vendor does not publish something, the cell says so rather than guessing.

Legend: **Y** = has it · **P** = partial · **N** = missing · **–** = not applicable to that business model

> **Category note on Juspay/Hyperswitch.** Juspay is a payment *orchestrator*, not an acquirer. It owns routing, retries, vault, reconciliation and a unified API; authorization, capture, settlement and disputes are pass-through to connector PSPs. Settlements, escrow, merchant KYC and reserves are **–** by architecture, not gaps. It is also the closest structural comparator to PayGate's gateway router.

---

## A. Payment methods

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Cards | Y (simulated) | Y | Y | Y | Y | Y | Y | Y |
| Network tokenization (RBI CoF) | N | Y | Y (TokenHQ) | Y | Y | Y | Y | Y (own token requestor) |
| UPI intent | Y | Y | Y | Y | **N** | Y | Y | Y |
| UPI QR | Y | Y | Y | Y | N | Y | Y | Y |
| UPI collect | N | Y | Y | P (deprecating, NPCI) | N | Y | P (ends 28 Feb 2026) | Y |
| UPI AutoPay / mandates | Y | Y | Y | P | N | Y | Y | P (generic mandate only) |
| Netbanking | Y (simulated) | Y (equivalents) | Y | Y | Y (equivalents) | Y (100+ banks) | Y (67 banks) | P |
| Wallets | Y (simulated) | Y | Y | Y | Y | Y | Y (8) | Y |
| EMI / installments | N | P | Y | P | N | P | Y (134 codes) | N |
| BNPL / pay-later | N | Y | Y | Y | Y | P (LazyPay only) | Y (4) | N |
| International cards | N | Y | P (activation-gated) | Y | Y | Y | Y | Y |
| Apple / Google Pay | N | Y (not in India) | – | Y | Y | – | – | P |

PayGate's methods are **simulator-backed**, not real rails. Sources: [Stripe](https://docs.stripe.com/payments/payment-methods/overview) · [Razorpay](https://razorpay.com/docs/payments/payment-methods/) · [Adyen](https://docs.adyen.com/payment-methods) · [Checkout.com](https://www.checkout.com/docs/payments/add-payment-methods) · [Cashfree](https://www.cashfree.com/docs/payments/manage/payment-methods) · [PayU](https://docs.payu.in/docs/card-type-codes-and-supported-banks-for-cards.md) · [Hyperswitch](https://api-reference.hyperswitch.io/v1/payments/payments--create)

---

## B. Money movement

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Auth/capture split | Y | Y | Y | Y | Y | Y | Y | Y |
| Partial capture | **N** | Y | **N** | Y | Y | Y | P (single only) | Y |
| Multi-capture | **N** | P (IC+ only, 50+1) | N | P (support-gated) | Y (`NonFinal`, 150 cap) | Y | N | Y (`manual_multiple`) |
| Void / cancel auth | Y (reverse-authorization) | Y | **N** (3-day auto-refund) | Y | Y | Y | Y | Y |
| Partial refunds | Y | Y | Y | Y | Y | Y | Y | Y |
| Instant refunds | N | N | Y (`speed=optimum`) | – | – | P (UPI only) | Y (IMPS push) | – |
| Disputes API + evidence | Y | Y | Y | Y | Y | Y | P (no accept endpoint) | P (unified, PSP adjudicates) |
| Settlements | Y | Y | Y (T+2) | Y | Y | Y (T+1/T+2) | Y (T+2) | – |
| On-demand / instant settlement | N | Y (Instant Payouts) | Y (T+0, ≤₹5 Cr) | Y | Y | Y (~15 min) | Y (~30 min) | – |
| Payouts | Y (simulated rail) | Y | Y (RazorpayX) | Y | Y | Y | Y | Y (pass-through) |
| Marketplace splits | Y (connected accounts) | Y | Y (Route) | Y | Y | Y (Easy Split) | Y | P (PSP-dependent) |
| Escrow | N | P | P | – | – | Y (OneEscrow) | N | N |
| Double-entry ledger | **Y** | not exposed | not exposed | not exposed | not exposed | not exposed | not exposed | Y (recon add-on) |

PayGate's **double-entry ledger is a genuine differentiator** — verified balanced under live traffic. No competitor exposes ledger primitives to merchants; Hyperswitch has one inside its paid Recon module.

Sources: [Stripe capture](https://docs.stripe.com/payments/capture-later) · [Stripe multicapture](https://docs.stripe.com/payments/multicapture) · [Razorpay capture](https://razorpay.com/docs/api/payments/capture/) · [Razorpay refund speed](https://razorpay.com/docs/payments/refunds/refund-speed/) · [Adyen capture](https://docs.adyen.com/online-payments/capture) · [Checkout capture](https://www.checkout.com/docs/payments/manage-payments/capture-a-payment) · [Cashfree pre-auth](https://www.cashfree.com/docs/payments/features/pre-authorisation) · [PayU pre-auth](https://docs.payu.in/docs/auth-and-capture-pre-authorize-card-payments.md) · [Hyperswitch overcapture](https://docs.hyperswitch.io/other-features/payment-orchestration/quickstart/overcapture.md)

---

## C. Merchant lifecycle

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Onboarding / KYC API | Y (simulated screening) | Y | Y | Y | Y | Y | Y (16-step, DigiLocker/VKYC) | – |
| Sub-merchants / connected accounts | Y | Y | Y | Y | Y | Y | Y | Y (org→merchant→profile) |
| Capabilities model | Y | Y | Y | Y | P | – | **N** | – |
| Reserves | Y (policy + escalations) | P | P | P | **Y** (`fixed`/`rolling` in balance) | N | N | – |
| **Signup authentication** | **N — unauthenticated** | Y | Y | Y | Y | Y | Y | Y |

`POST /v1/merchants` requires no authentication (see `issues.md`). Every comparator gates merchant creation. This is the single clearest correctness gap in the lifecycle area.

Sources: [Stripe capabilities](https://docs.stripe.com/connect/account-capabilities) · [Checkout reserves](https://www.checkout.com/docs/platforms/for-payfac/manage-sub-entity-balances) · [Adyen LEM](https://docs.adyen.com/platforms/onboard-users) · [PayU partner API](https://docs.payu.in/reference/partner-integration-api-introduction.md)

---

## D. Developer surface

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Server SDK languages | **2** (Go, JS) | 7 | 7 | 7 | 7 | 6 | P (3 documented) | P (Node only) |
| Hosted checkout | Y | Y | Y | Y | Y | Y | Y | Y |
| Embedded / drop-in | P (sandbox page) | Y | Y | Y | Y | Y | Y | Y |
| Webhooks | Y | Y | Y | Y | Y | Y | P (4 inconsistent contracts) | Y |
| **Published retry schedule** | **N** | Y (backoff to 3 days) | P (24 h envelope) | **Y (9s…8h, 30 days)** | Y (8 tries, ~30 h, then cancel) | Y (3 @ 2/10/30 min, configurable) | P (varies by product) | **Y (16 tries / 24 h)** |
| Signature scheme | Y (HMAC-SHA256 + RFC 9421) | Y | Y | Y | Y | Y | P | Y (HMAC-SHA512) |
| **Documented delivery guarantee** | **N** | Y (at-least-once, unordered) | Y | P | **Y (explicit)** | Y | **N** | **Y (explicit)** |
| Idempotency | Y (`Idempotency-Key`, 409 on conflict) | Y (24 h) | P (payouts only) | Y (64 char) | Y (24 h TTL) | Y | **N** | P (client-supplied id) |
| **Published idempotency TTL** | **N** | Y (≥24 h prunable) | – | N | Y (24 h) | N | – | – |
| Pagination | P (cursor on orders, offset elsewhere) | Y (cursor) | Y (offset) | Y | Y | P | P | Y |
| **API versioning** | **N** (`/v1` path only) | Y (dated header + named releases) | P (`/v1`, policy only) | **Y (URL-pinned + diff tool)** | P (compat policy only) | Y (`x-api-version`, dated) | Y (v1/v2) | Y (v1/v2) |
| Sandbox + test credentials | Y | Y | Y | Y | Y | Y | P | Y |
| CLI | **N** | Y | Y | N | N (MCP server) | N (MCP server) | Y | N |
| **Published rate limits** | **N** | Y (100 rps live / 25 sandbox) | **N** | P (LEM only) | Y (100 rps read + 100 write) | Y (per-endpoint) | **N** | Y (80 rps) |

PayGate's idempotency implementation is genuinely strong — verified under 8 parallel identical requests producing exactly one order, with `409 IDEMPOTENCY_CONFLICT` on key reuse with a different body. That is better than Razorpay's (payouts only) and PayU's (none). **It is simply undocumented**, as are the retry schedule, delivery guarantee and rate limits.

Sources: [Stripe webhooks](https://docs.stripe.com/webhooks) · [Stripe idempotency](https://docs.stripe.com/api/idempotent_requests) · [Stripe versioning](https://docs.stripe.com/api/versioning) · [Stripe rate limits](https://docs.stripe.com/rate-limits) · [Adyen webhook retries](https://docs.adyen.com/development-resources/webhooks/troubleshoot) · [Adyen versioning](https://docs.adyen.com/development-resources/versioning) · [Checkout webhooks](https://www.checkout.com/docs/developer-resources/event-notifications/receive-webhooks) · [Checkout idempotency](https://www.checkout.com/docs/developer-resources/api/idempotency) · [Checkout rate limits](https://www.checkout.com/docs/developer-resources/api/api-rate-limits) · [Razorpay webhooks](https://razorpay.com/docs/payments/dashboard/settings/webhooks/) · [Cashfree rate limits](https://www.cashfree.com/docs/api-reference/payments/rate-limits) · [Hyperswitch webhooks](https://docs.hyperswitch.io/integration-guide/webhooks.md)

---

## E. Operational

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Dashboard | Y (23 routes) | Y | Y | Y | Y | Y | Y | Y |
| Reports / exports | Y | Y | Y | Y | Y | Y | P | Y |
| Reconciliation | Y (3-way, mismatch workflow) | Y | Y | **Y (per-transaction settlement detail)** | Y (balance-centric) | Y | P | Y (paid add-on) |
| RBAC | Y | Y | Y | Y | Y | Y | P | Y |
| Audit trail | **Y** | P (180-day retention) | P | not verified | Y | P (API logs only) | **N** | P |
| Alerting | P (Prometheus/Alertmanager) | Y | P | not verified | P | Y (success-rate, latency) | **Y (Overwatch)** | N |
| **Dashboard write coverage** | **P** | Y | Y | Y | Y | Y | Y | Y |

PayGate's audit trail and three-way reconciliation are stronger than several incumbents. The gap is that several dashboard pages are **read-only despite the backend exposing writes** — webhooks, settlements, recon, mandates, and capture/refund actions (`issues.md`).

Sources: [Adyen settlement detail](https://docs.adyen.com/reporting/settlement-reconciliation) · [Checkout reconciliation](https://www.checkout.com/docs/funds-management/reconcile-with-checkout-com) · [PayU Overwatch](https://docs.payu.in/docs/payu-monitoring-alerts-overwatch.md) · [Stripe teams](https://docs.stripe.com/get-started/account/teams)

---

## F. Trust and compliance

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| PCI DSS certification | **N** (design-aligned only) | Y (L1 SP) | Y (L1) | Y | Y | Y (v4.0.1 L1) | P (marketing only) | P (cloud only) |
| No raw PAN in API | **Y** | Y | Y | Y | Y | Y | Y | Y |
| 3DS2 / SCA | P (simulated challenge) | Y | Y | Y | Y | Y | Y | Y |
| **SCA exemption handling** | **N** | Y (TRA, low-value, MIT, data-only) | – | **Y (4-value enum)** | **Y (6-value enum)** | not documented | – | Y (decision manager) |
| Tokenization / vault | Y (own vault) | Y | Y | Y | Y | Y | Y | Y |
| Fraud / risk engine | Y (rules + scoring) | Y (Radar) | P (no merchant rules) | Y (Protect) | Y | Y (RiskShield) | N (documented) | P (FRM connectors) |
| **Velocity rules** | **Y** (configurable) | P | P (internal only) | P | P | **Y (Smart Rules)** | N | N |
| Public status page | **N** | Y | Y | Y | **N (login-gated)** | Y | **N (404)** | Y |
| **Published uptime SLA** | **N** | N (marketing only) | P (blog only) | **Y (99.9% quarterly, contractual)** | N | N | N | P (cloud tier claim) |

Two observations worth carrying into positioning:

1. **SCA exemption handling is the sharpest compliance gap.** Adyen and Checkout.com both expose an explicit exemption enum; PayGate has none. This matters more than any latency work — it directly affects authorization rates and liability shift in Europe.
2. **A published uptime number would beat most of this field.** Only Adyen publishes a contractual figure. Checkout.com's status page is login-gated and PayU has none at all.

Sources: [Adyen SCA options](https://docs.adyen.com/online-payments/psd2-sca-compliance-and-implementation-guide/sca-options) · [Checkout 3DS exemptions](https://www.checkout.com/docs/risk-management/3d-secure/sessions/3ds-exemptions) · [Stripe SCA exemptions](https://docs.stripe.com/payments/3d-secure/strong-customer-authentication-exemptions) · [Adyen T&Cs](https://www.adyen.com/legal/terms-and-conditions) · [Cashfree Smart Rules](https://www.cashfree.com/docs/payments/risk-shield/smart-rules)

---

## G. Performance and scale

Competitor figures are **documented**; PayGate figures are **measured** (see `PERFORMANCE-BASELINE.md`).

| | PayGate (measured) | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Published rate limit | none | 100 rps live / 25 sandbox | not published | LEM only (700/5s) | 100 rps read + 100 write | per-endpoint (200/min create order) | not published | 80 rps |
| **Effective rate limit** | **10 rps per key per path** | — | — | — | — | — | — | — |
| Measured saturation | **~75 req/s** | not published | not published | not published | not published | not published | not published | docs claim ~10k TPS |
| Peak throughput disclosed | — | not published | 200–1,500 rps (IPL, blog) | not published | not published | 100k txn/min (marketing) | not published | 50k TPS (Juspay corp) |
| Latency targets | p50 69–170 ms, p99 0.6–3.2 s | not published | not published | not published | not published | not published | not published | not published |
| Webhook delivery lag | **p50 224 s, p99 578 s** | — | — | — | — | — | — | — |
| Settlement timeline | simulated | T+2 US / T+3 UK-EU | T+2, T+0 instant | not published | account-specific | T+1/T+2, ~15 min instant | T+2, 15 min priority | – |
| Uptime SLA | none | none published | blog only | **99.9% quarterly** | none published | none published | none | cloud claim |

**Nobody in this field publishes latency percentiles.** That is an opportunity, not just a gap: PayGate already measures them, and publishing a real p99 with methodology would be genuinely differentiating — but only once the numbers are defensible.

---

## Where PayGate genuinely leads

Not marketing claims — each verified in this repository or by measurement:

1. **Double-entry ledger exposed as a first-class concept**, balanced to zero across every transaction under live traffic. No competitor exposes this to merchants.
2. **Idempotency semantics** stronger than Razorpay's or PayU's — verified: 8 parallel identical requests create exactly one order; key reuse with a different body returns `409`.
3. **Three-way reconciliation** with a mismatch workflow, comparable to Adyen's and ahead of PayU's.
4. **Merchant-configurable velocity rules**, which Stripe, Adyen and Checkout.com only offer partially.
5. **Transactional outbox** as an explicit architectural commitment — the *design* is right even though the *implementation* currently drains at 1 event/sec.
6. **Cross-tenant isolation** verified by adversarial testing rather than asserted.

## Where the gaps are structural, not incidental

* **No real acquiring rails.** Every method is simulator-backed. This is a category difference, not a feature gap, and it bounds any comparison above.
* **No PCI DSS certification.** The design avoids raw PAN, which is the hard part, but certification is an audit and a compliance programme.
* **No published operational contract** — no rate limits, no retry schedule, no delivery guarantee, no SLA, no status page. Competitors compete on these documents as much as on code, and PayGate's underlying behaviour is often already good enough to publish.
