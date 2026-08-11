# PayGate — Competitive Feature Matrix

Compiled 2026-08-11 against commit `bf61c16`. Competitor rows are **documented** — taken from each vendor's official documentation, with a source link. PayGate rows are **measured or read from this repository**. Where a vendor does not publish something, the cell says so rather than guessing.

Legend: **Y** = has it · **P** = partial · **N** = missing · **–** = not applicable to that business model

This document is in two parts. **Part I (A–G)** compares payment acceptance — the surface PayGate already occupies. **Part II (H–T)** maps everything else these companies sell, because acceptance is no longer where most of them make their margin, and a foundation should be shaped for the whole surface rather than the overlapping slice. `FEATURES.md` turns Part II into an actionable, exhaustive catalogue.

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

# Part II — The wider product surface

Sections A–G compare **payment acceptance**. That was the wrong frame for a foundation decision. None of these companies sells only acceptance any more, and several make more margin outside it. Sections H–T below map the rest of the surface, because the question "what should PayGate's foundation be shaped for?" is answered by the *whole* competitor surface, not the part PayGate already overlaps.

Two structural corrections to Part I, both from vendor docs:

* **Adyen's `RevenueAccelerate` and `RevenueProtect` are legacy names.** The current products are **Adyen Uplift** (five modules: Personalize, Tokenize, Protect, Authenticate, Optimize) and **Protect**. Classic risk rules do not auto-migrate to the new engine. [Uplift](https://docs.adyen.com/uplift)
* **Stripe's Radar is now four tiers** (Lite / Standard / Plus / Pro), not "Radar + Radar for Fraud Teams". Chargeback Protection has disappeared from the current plan comparison — status unconfirmed. In India, Radar's rules, lists, review queue and threshold editing are **disabled**. [Radar](https://docs.stripe.com/radar/how-radar-works)

---

## H. Billing, subscriptions and revenue operations

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Subscriptions | Y (fixed-amount, day/week/month) | Y | Y | – | N (API primitive only) | Y | Y (Zion) | P (orchestrates Chargebee/Recurly/Stripe) |
| Subscription schedules / phases | **N** | Y (10 phases, per-phase proration) | N | – | N | N | P | N |
| Usage-based / metered billing | **N** | Y (Billing Meters + **Metronome**) | N | – | N | N | N | N |
| Tiered / graduated / volume pricing | **N** | Y | P | – | N | N | P | N |
| Entitlements (feature access from plan) | **N** | Y | N | – | N | N | N | N |
| Credit grants / prepaid balances | **N** | Y (append-only ledger) | N | – | N | N | P (Merchant Wallet) | N |
| Coupons / promo codes / offers engine | **N** | Y (20 stackable) | Y (Offers) | – | N | Y (Offers) | **Y (Offer Engine + Loyalty Edge)** | **Y (PSP-agnostic Offers)** |
| Trials with reminder emails | **N** | Y | P | – | N | N | N | N |
| Self-serve customer portal | **N** | Y (+ cancellation deflection survey) | P | – | N | N | N | N |
| Dunning / smart retries | **P** (fixed `RetryIntervalHours`) | **Y (ML-timed, up to 8 attempts)** | P | Y (Auto Rescue, incl. SEPA DD) | N | N | P | **Y (Revenue Recovery, 20–30 signals)** |
| Card account updater | **N** | Y | P (via TokenHQ) | **Y (real-time, in-transaction)** | **Y (bundled, not an upsell)** | P | P | Y |
| Invoicing (standalone, GST / e-invoice) | P (invoice records, no PDF or e-invoice) | Y (+ EU e-invoicing) | **Y (GST-compliant)** | – | N | N | Y | N |
| Quotes | **N** | Y (revisions + PDF) | N | – | N | N | N | N |
| Revenue recognition / accrual accounting | **N** | **Y (per-second amortization, trial balance, ASC 606)** | N | N | N | N | N | N |
| Billing analytics (MRR / churn / LTV / cohorts) | **N** | Y (new/expansion/contraction/churn/FX) | P | – | N | N | P | P (Insights) |
| Token / LLM metering | **N** | P (AI Gateway, private preview) | N | N | N | N | N | N |

Sources: [Stripe Billing](https://docs.stripe.com/billing/subscriptions/overview) · [Metronome](https://docs.stripe.com/billing/how-metronome-works-with-stripe) · [Entitlements](https://docs.stripe.com/billing/entitlements) · [Revenue Recognition](https://docs.stripe.com/revenue-recognition/get-started) · [Adyen Auto Rescue](https://docs.adyen.com/online-payments/auto-rescue) · [Checkout RTAU](https://checkout.com/products/real-time-account-updater) · [Razorpay Invoices](https://razorpay.com/docs/payments/invoices/) · [PayU Zion](https://docs.payu.in/docs/using-zion-subscription-automation-platform) · [Hyperswitch Revenue Recovery](https://hyperswitch.io/revenue-recovery)

**Read on PayGate.** `internal/billing` defines `Customer`, `Subscription`, `Invoice`, `InvoiceAttempt`, `PaymentLink`, `VirtualAccount`, `InboundCollection`, `ConnectedAccount`, `SplitInstruction` and `UPIMandate`. A subscription is a **single fixed amount on a day/week/month interval** with a fixed retry count and interval (`internal/billing/domain.go:143-149`). There is no price object, no quantity, no proration, no metering, no discount. This is a *collection scheduler*, not a billing engine — a defensible scope, but it should be named as such rather than compared to Stripe Billing.

Revenue recognition is the largest uncontested space in this table: **only Stripe has it.** It is also the one that maps most naturally onto PayGate's existing double-entry ledger, since accrual accounting is exactly what a ledger is for.

---

## I. Tax and compliance automation

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Tax calculation at checkout | **N** (tax *profile* record only) | **Y (100+ countries, 600+ codes)** | P (GST on invoices) | N | N | N | P | P (TaxJar connector) |
| Nexus / threshold monitoring | **N** | Y | N | N | N | N | N | N |
| Tax registration management | **N** | Y (+ register-on-your-behalf) | N | N | N | N | N | N |
| Filing and remittance | **N** | Y (TaxJar, 46 US states; Taxually global) | N | N | N | N | N | N |
| Tax IDs / reverse charge / exemptions | **N** | Y | P | N | N | N | P | N |
| Standalone tax API for non-native flows | – | Y | – | – | – | – | – | – |

Sources: [Stripe Tax](https://docs.stripe.com/tax/supported-countries) · [Stripe filing](https://docs.stripe.com/tax/filing) · [Hyperswitch tax providers](https://docs.hyperswitch.io/llms.txt)

`GET/PUT /v1/merchants/{id}/tax-profile` stores tax metadata. It does not calculate anything. **Stripe is effectively unopposed in this column** — no Indian competitor sells tax automation, and neither Adyen nor Checkout.com does either.

---

## J. Issuing (card programmes)

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Virtual card issuing | **N** | Y | P (Corporate CC via RazorpayX) | Y | Y | N | **Y (Virtual Cards)** | N |
| Physical card issuing | **N** | Y (plastic / metal, bulk ship) | P | Y | Y | N | P | N |
| Spend controls (MCC, velocity, limits) | **N** | Y | N | Y (transactionRules) | Y (`/controls`) | N | P | N |
| Real-time auth decisioning webhook | **N** | **Y (2 s budget + Autopilot fallback)** | N | **Y (2000 ms + fallback)** | Y | N | N | N |
| 3DS on issued cards | **N** | Y | N | Y (OTP or out-of-band SDK) | Y | N | P (Wibmo ACS) | N |
| Push provisioning to Apple / Google Pay | **N** | Y | N | P | **Y (Card Management SDK)** | N | P (Wibmo Token Hub) | N |
| PIN reveal / PAN reveal SDK | **N** | Y | N | Y (AES + ISO-4 PIN block) | Y | N | Y | N |
| Consumer credit issuing (bureau reporting) | **N** | P (private preview, US) | N | N | N | N | P (LazyPay / PayU Finance) | N |
| Issuing for AI agents | **N** | P (public preview) | N | N | N | N | N | N |

Sources: [Stripe Issuing](https://docs.stripe.com/issuing/global) · [Stripe real-time auth](https://docs.stripe.com/issuing/controls/real-time-authorizations) · [Adyen Issuing](https://docs.adyen.com/issuing) · [Checkout Issuing](https://www.checkout.com/docs/card-issuing) · [PayU Virtual Cards](https://docs.payu.in/docs/virtual-cards-introduction) · [Wibmo Issuer Token Hub](https://www.wibmo.com/issuer-token-hub)

Issuing is **licence-gated everywhere** — BIN sponsorship, scheme membership, a partner bank — and access is sales-gated even at Stripe and Adyen. It is the clearest example of a category PayGate can model architecturally but cannot ship.

The transferable idea is the **synchronous authorization webhook with a hard timeout and a declarative fallback**. Stripe gives 2 s, Adyen 2000 ms, and both fall back to stored rules on timeout or error. PayGate's risk engine already evaluates rules inline; what is missing is the *contract* — a bounded budget, a deterministic fallback, and no silent failure.

---

## K. In-person, POS and unified commerce

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Terminal hardware | **N** | Y (S700/S710/WisePOS/Verifone) | **Y (Ezetap: All-in-One/Smart/Mini)** | **Y (S1E/M450/P630/UX410)** | **N** | P (SoftPOS only) | Y | **N** |
| Terminal API / protocol | **N** | Y (server-driven + SDKs) | Y | Y (nexo Retailer v3.0, local + cloud) | N | Y (terminal + QR APIs) | P | N |
| Tap to Pay on iPhone / Android | **N** | Y (8+ markets) | P | Y (12+ markets, entitlement-gated) | N | Y (Android) | P | N |
| Offline / store-and-forward | **N** | Y (mobile SDKs only) | P | Y (with Auto Rescue retries) | N | P | P | N |
| Fleet / terminal management API | **N** | Y (Locations, OTA, Hardware Orders) | Y | Y (4-level settings hierarchy) | N | P | P | N |
| Soundbox / merchant audio confirmation | **N** | N | **Y (Tap & Scan, Bharat, Signature)** | N | N | N | N | N |
| Unattended / autonomous stores | **N** | P | N | **Y (entry pre-auth, exit capture)** | N | N | N | N |
| Cross-channel shopper identity | **N** | P (Link) | N | **Y (ShopperDNA, PAR, card alias)** | N | P (Customer Hub) | N | N |
| Buy-online-return-in-store (BORIS) | **N** | N | N | **Y (referenced refunds)** | N | N | N | N |
| Endless aisle | **N** | N | N | Y | N | N | N | N |
| Card-as-loyalty-identifier | **N** | N | P (Engage) | **Y (payment-linked loyalty)** | N | N | P (Loyalty Edge) | P (Offers) |
| US debit least-cost routing (in-person) | **N** | P | – | **Y (AID selection rules)** | N | – | – | – |

Sources: [Adyen POS](https://docs.adyen.com/point-of-sale/what-we-support/select-your-terminals) · [Adyen unified commerce](https://docs.adyen.com/unified-commerce/collect-data) · [Stripe Terminal](https://docs.stripe.com/terminal/payments/setup-reader) · [Razorpay POS](https://razorpay.com/pos/) · [Cashfree SoftPOS](https://www.cashfree.com/docs/payments/softpos/introduction)

**This is Adyen's moat and Checkout.com's biggest hole** — Checkout.com has no in-person product at all. In India, POS acquiring additionally requires an **RBI PA-P licence**; Razorpay only received one on 22 Jan 2026, three years after its online PA.

The architecturally interesting item is not the hardware, it is **cross-channel identity**. Adyen's `shopperEmail` / PAR / card-alias join key is what makes BORIS, endless aisle and card-as-loyalty possible at all. PayGate already computes `fingerprintHash(merchantID, pan, expMonth, expYear)` in `internal/tokenization/service.go:272` — that is a cross-session join key, and it is currently unused as one.

---

## L. Banking, treasury and embedded finance

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Business bank / current accounts | **N** | Y (Treasury, US + GB) | **Y (RazorpayX)** | Y (22 countries, "in development") | N | N | N | N |
| Virtual accounts for collections | **Y** | P (Financial Connections) | **Y (Smart Collect 2.0)** | Y | N | **Y (Auto Collect)** | P | N |
| Escrow | **N** | P | Y (Escrow+) | – | N | **Y (OneEscrow)** | N | N |
| Sweeps / auto top-up rules | **N** | Y (auto transfer rules) | P | **Y (push/pull by schedule or threshold)** | N | P (low-balance queuing) | N | N |
| Vendor payments / AP automation | **N** | P | **Y (+ auto monthly TDS)** | N | N | N | N | N |
| Payroll | **N** | P (partners) | **Y (XPayroll: TDS/PF/ESI/PT)** | N | N | N | N | N |
| Tax payments / statutory filing | **N** | N | **Y** | N | N | N | N | N |
| Corporate cards | **N** | Y (Issuing) | Y | Y | Y | N | Y | N |
| Multi-currency settlement | **N** | Y (18 currencies) | P | Y | **Y (20 currencies, held FX rate)** | Y | P | – |
| FX rate locks | **N** | P (FX Quotes API, preview) | N | N | **Y (capture to settlement hold)** | N | P (DCC) | Y (payouts FX) |
| Cross-border collections (export) | **N** | Y | Y (MoneySaver, 130+ ccy) | Y | Y | **Y (Global Collections, 17+ ccy, FIRA)** | Y | Y |
| Co-lending / three-ledger disbursal | **N** | N | P (Digital Lending 2.0) | N | N | **Y** | N | N |

Sources: [Stripe Treasury](https://docs.stripe.com/treasury) · [Adyen business accounts](https://docs.adyen.com/business-accounts) · [Adyen sweeps](https://docs.adyen.com/platforms/top-up-balance-account) · [RazorpayX](https://razorpay.com/x/) · [Cashfree OneEscrow](https://www.cashfree.com/docs/payouts/oneescrow/overview) · [Checkout Treasury and FX](https://checkout.com/products/treasury-fx)

**Non-banks cannot hold deposits.** Every "business account" above is a software layer over a partner bank. That makes the *software* — balance accounts, sweep rules, approval workflows, multi-ledger allocation — the actual product, and PayGate's ledger is the right substrate for it. Sweeps in particular (push/pull by schedule or balance threshold) are pure ledger logic with no licence requirement until money actually moves.

---

## M. Capital and lending

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Merchant cash advance / revenue-based finance | **N** | Y (6 countries) | **Y (Cash Advance, ₹50k–₹10L)** | **Y (Capital, 9 markets)** | **N** | N | Y | N |
| Line of credit | **N** | P (preview) | Y | N | N | N | Y | N |
| Settlement-as-credit | **N** | P (Instant Payouts) | **Y** | N | N | Y (Instant Settlements) | Y | N |
| Capital-for-platforms (embed lending) | **N** | P (public preview) | Y (Digital Lending 2.0) | N | N | P (Co-Lending) | N | N |
| Consumer BNPL on own book | **N** | N | N | N | N | N | **Y (LazyPay, own NBFC)** | N |
| Embedded credit at checkout (multi-lender) | **N** | P | Y (EMI²) | N | N | Y (BNPL Plus) | Y | **Y (HyperCredit, 20+ lenders)** |

Sources: [Adyen Capital](https://docs.adyen.com/capital) · [Stripe Capital](https://docs.stripe.com/capital/how-capital-for-platforms-works) · [Razorpay Cash Advance](https://razorpay.com/newsroom/struggling-to-get-a-business-loan-now-borrow-money-instantly-with-razorpays-cash-advance-credit-line/) · [Juspay HyperCredit](https://juspay.io/in/hypercredit) · [LazyPay](https://lazypay.in/pay-later)

**Only PayU lends off its own balance sheet** (PayU Finance India, NBFC acquired 2018). Razorpay's NBFC application was **rejected by the RBI**, so Capital is a partner-NBFC marketplace. Cashfree has no lending entity at all. Checkout.com has no lending product.

For a foundation this splits cleanly: the *underwriting input* is processing history, which PayGate already holds in the ledger, and the eligibility and offer-management layer is unblocked. The *disbursal* is licence-gated and is not.

---

## N. Identity, KYC and verification as a product

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Sold as a standalone product | **N** | Y (Identity) | P | **N** | **Y (+ AML screening)** | **Y (Secure ID, 19+ APIs)** | P | N |
| Document verification | **N** | Y (120+ countries) | P | Y (internal to LEM) | Y (3,000+ doc types) | Y | Y | N |
| Biometric liveness / face match | **N** | Y (selfie) | P | N | Y | **Y** | P (Wibmo) | N |
| Video KYC | **N** | N | P | N | Y | **Y (AI, multilingual)** | Y | N |
| Bank account verification (penny drop) | **N** | Y (Financial Connections) | Y | N | N | **Y (+ reverse penny drop)** | Y | N |
| UPI VPA verification | **Y** | – | Y | – | – | Y | Y | Y |
| Government-ID APIs (PAN/Aadhaar/GST/CIN) | **N** | – | P | – | – | **Y (full suite + DigiLocker + eSign)** | Y (CKYC) | N |
| AML / sanctions / PEP screening | **P (simulated)** | P | P | Y (internal) | **Y (ComplyAdvantage)** | P | P | N |
| Hosted onboarding link flow | P | Y | Y | **Y (37 countries, 4-min single-use links)** | Y | Y (1-Click Onboarding SDK) | Y (16-step) | – |

Sources: [Cashfree Secure ID](https://www.cashfree.com/docs/secure-id/kyc-stack/overview) · [Checkout IDV](https://checkout.com/products/identity-verification) · [Stripe Identity](https://docs.stripe.com/identity/verification-checks) · [Adyen hosted onboarding](https://docs.adyen.com/platforms/onboard-users)

**PayGate already ships VPA verification** (`internal/upiverify`) — one API out of the 19 Cashfree sells as a separate revenue line. The *shape* of a verification product already exists in the codebase. Verification APIs are also, per the licensing research below, **not licence-gated**, unlike almost everything else in this document.

Adyen has no merchant-facing IDV product; its verification is internal to Legal Entity Management. Checkout.com and Cashfree both sell it standalone.

---

## O. Authorization-rate optimisation and network services

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Named optimisation product | **N** | **Y (Authorization Boost)** | P (Optimizer is routing) | **Y (Uplift)** | **Y (Intelligent Acceptance)** | P (FlowWise) | P (Wibmo Orchestrator) | **Y (Decision Engine)** |
| Network tokenization | **P (field only, no cryptogram)** | Y | Y (TokenHQ) | Y (Adyen-acquired only) | Y (+ merchant-managed cross-PSP) | Y (+ BYO TRID) | Y (Token Hub) | **Y (TR *and* TSP, 150M+ tokens)** |
| Co-badged debit tokens (least-cost) | **N** | P | **Y (Accel/Maestro/NYCE/PULSE/STAR)** | – | N | – | – | P |
| Real-time account updater | **N** | Y | P | **Y (synchronous, in-transaction)** | Y | P | P | Y |
| Issuer-preference message reformatting | **N** | Y (Adaptive Acceptance) | N | Y (Smart Payment Messaging) | Y (Dynamic Payload Optimization) | N | N | Y |
| Decline-code-conditioned retries | **P (failover on error only)** | Y | P | **Y (Auto Rescue, ML-timed)** | Y (Intelligent Retries) | P | P | **Y (20–30 signals)** |
| PINless debit routing | **N** | Y (US) | – | Y | Y | – | – | P |
| Success-rate-based dynamic routing | **N** | P | Y | Y | Y (Strategic Routing) | Y (AI routing) | Y | **Y (SR ranking + autopilot)** |
| Cost-aware / least-cost routing | **N** | P (IC+ reporting) | P | Y | Y | Y | Y | **Y (multi-objective re-ranking)** |
| PSP downtime detection + auto-cascade | **P (circuit breaker)** | N | P | N | N | Y | **Y (Overwatch)** | **Y (Outage Alerts)** |
| A/B testing on live traffic | **N** | P (preview) | Y (HyperCheckout) | **Y (Uplift Experiments, 50/50)** | Y (shadow testing) | N | N | Y |
| Adaptive / risk-based 3DS | **N** | Y (Adaptive 3DS, Radar Plus) | P | P (limited availability, Jul 2026) | Y (Smart Authentication) | P | Y (Wibmo RBA) | Y (3DS decision manager) |
| SCA exemption engine | **N** | Y | – | Y | Y (6-value enum) | N | – | Y |
| Standalone / acquirer-agnostic 3DS | **N** | Y | N | Y (not with `/sessions`) | **Y (`/sessions`)** | N | **Y (Wibmo 3DSS)** | Y (Juspay 3DS Server) |
| Interchange / cost observability | **N** | P (IC+ US/CA only) | N | N | N | N | N | **Y (downgrade detection, fee audit)** |
| Chargeback auto-defense | **N** | Y (Smart Disputes) | P | **Y (unprompted)** | P (Verifi RDR) | N | P | Y |

Sources: [Stripe Authorization Boost](https://docs.stripe.com/payments/analytics/optimization) · [Adyen Uplift](https://docs.adyen.com/uplift) · [Adyen network tokens](https://docs.adyen.com/online-payments/network-tokenization) · [Adyen RTAU](https://docs.adyen.com/online-payments/account-updater/real-time-account-updater) · [Checkout Intelligent Acceptance](https://checkout.com/products/intelligent-acceptance) · [Hyperswitch Decision Engine](https://github.com/juspay/decision-engine) · [Hyperswitch Cost Observability](https://hyperswitch.io/cost-observability) · [Juspay tokenisation](https://juspay.io/tokenisation)

**This is the densest column of competitor investment in the entire document, and PayGate's thinnest row.** Every competitor has a *named, branded* optimisation product. PayGate has a gateway router with a circuit breaker and `FailoverOnError`.

`internal/tokenization` stores a `network_token_type` field that flips from `pan_token` to `network_token` when a `networkReference` is present (`service.go:293`). That is a **placeholder**, not network tokenization — there is no cryptogram, no token requestor certification, no TSP relationship. It is marked **P** above for that reason and should not be counted as a capability.

Two items in this table need no licence, no scheme membership and no partner: **decline-code-conditioned retry policy** and **outcome-feedback routing**. Both are pure logic over data PayGate already records.

---

## P. Orchestration, vault portability and multi-PSP

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Multi-PSP orchestration | **P (internal router only)** | P (Orchestration, private preview) | **Y (Optimizer)** | N | N | **Y (FlowWise, 40+ incl. rivals)** | Y (Wibmo Orchestrator) | **Y (300+ PSPs)** |
| Forward vaulted credentials to third parties | **N** | P (support-gated, ~25 destinations) | N | P (support-gated, region-matched) | **Y (`POST /forward`, arbitrary endpoints)** | N | P (PayU Vault) | **Y (decoupled vault)** |
| Merchant-managed / BYO token vault | **N** | N | N | N | Y ("launching soon") | Y (BYO TRID) | Y (3 models) | **Y (7 deployment models)** |
| PAN import from a prior processor | **N** | P (support-mediated, ~10 days) | P | P | P | P | P | Y (migration tooling) |
| Open-source core | **N** | N | N | N | N | N | N | **Y (Apache 2.0, Rust, 43k stars)** |
| Self-hostable | **Y** | N | N | N | N | N | N | Y |
| Connector SDK / bring-your-own-PSP | **P (internal interface)** | N | N | N | N | N | N | **Y (90+ self-hosted, 210+ cloud)** |

Sources: [Checkout Forward](https://www.checkout.com/docs/payments/store-and-manage-credentials/forward-stored-credentials) · [Stripe Vault and Forward](https://docs.stripe.com/payments/vault-and-forward) · [Hyperswitch](https://github.com/juspay/hyperswitch) · [Cashfree FlowWise](https://www.cashfree.com/docs/payments/flowwise/overview) · [Razorpay Optimizer](https://razorpay.com/docs/payments/optimizer/)

**Orchestration is the one large product area that does not require a payment-aggregator licence**, provided you never touch merchant funds. Juspay built a $1.2B business as a technology service provider before receiving its PA authorisation in Feb 2024. For a project that cannot obtain a licence, this is the most strategically available category in the document — and `internal/gateway` is already the right shape for it.

Note also that Cashfree's FlowWise routes across **Razorpay, PayU and Paytm**. Orchestration is where these companies compete for each other's traffic.

---

## Q. Agentic commerce and AI surfaces

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| MCP server | **N** | Y (hosted, OAuth) | Y | P (alpha, local) | **Y (hosted, OAuth, no API keys)** | Y (PG / Payouts / SecureID) | Y (remote + builder) | Y |
| Agent-scoped payment tokens | **N** | **Y (Shared Payment Tokens)** | P (UPI Reserve Pay) | Y | Y (delegated payment tokens) | Y (Reserve Pay + device tokens) | Y (UPI OTM + passkeys) | P |
| Protocol support | **N** | UCP, ACP, MPP, x402 | — | **UCP, AP2, ACP** | ACP | — | — | stated direction, not shipped |
| Agent spend controls / approvals | **N** | Y (Agent Guardrails, preview) | P | N | N | N | N | N |
| Agent-facing catalogue / feed | **N** | Y (Agentic Commerce Suite) | N | **Y (Agentic Feed + Cart)** | N | N | N | N |
| Machine-to-machine micropayments | **N** | Y (MPP; card min $0.50 or stablecoin) | N | N | N | N | N | N |
| Agent skills / `llms.txt` / CLI | **N** | Y (`stripe agent setup`) | Y (llms.txt, n8n) | N | N | **Y (agent-skills CLI incl. rival migration guides)** | Y (CLI, Jun 2026) | N |
| Cards issued to agents | **N** | P (preview) | N | N | N | N | N | N |
| In-dashboard agentic execution | **N** | Y (Console preview, Workflows GA) | P (Agent Studio) | N | N | N | P | N |

Sources: [Stripe agentic](https://docs.stripe.com/agentic-commerce) · [Shared Payment Tokens](https://docs.stripe.com/agentic-commerce/concepts/shared-payment-tokens) · [Adyen Agentic](https://www.adyen.com/press-and-media/adyen-agentic) · [Checkout MCP](https://github.com/checkout/checkout-mcp) · [Cashfree agent-skills](https://github.com/cashfree/agent-skills) · [Razorpay agentic](https://razorpay.com/agentic-payments/) · [PayU agentic](https://docs.payu.in/docs/agentic-commerce)

**Every competitor in this table ships an MCP server. PayGate has none.** This is the lowest-effort, highest-visibility gap in the document: an MCP server over an existing REST API is a wrapper, not a licence application.

The deeper primitive is the **scoped, single-use payment grant** — a token carrying a currency, an amount ceiling, an expiry and a merchant scope, spendable by a third party without full credential access. India's equivalent already exists on public rails as **UPI Reserve Pay / one-time mandate** (block now, debit later), which Razorpay, Cashfree and PayU all expose. PayGate already has `UPIMandate` with `AmountLimit`, `IntervalUnit` and `RetryWindowHours` — the block-and-debit primitive exists; what is missing is the agent-facing grant semantics on top of it.

---

## R. Stablecoins and digital assets

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| Stablecoin acceptance | **N** | Y (US GA; EU/HK/MX/CH preview) | N | **N (none found)** | Y (Coinbase Payments, enterprise) | N | N | N |
| Stablecoin settlement to merchant | **N** | Y | N | N | **Y (Fireblocks; US ex NY/VA/AK)** | N | N | N |
| Stablecoin payouts | **N** | Y (~170 countries) | N | N | N | N | N | N |
| Stablecoin-denominated financial accounts | **N** | Y (USDC + Bridge USDB, 8+ chains) | N | N | N | N | N | N |
| Own stablecoin issuance | **N** | Y (Bridge Open Issuance) | N | N | **Y (Blue EMI, euro stablecoin licence)** | N | N | N |
| Own settlement blockchain | **N** | Y (Tempo) | N | N | N | N | N | N |
| Crypto on-ramp (fiat to crypto) | **N** | Y (Stripe is merchant of record) | N | N | N | N | N | N |
| Stablecoin-backed issued cards | **N** | P (private preview, 30+ markets) | N | N | N | N | N | N |
| x402 / agent micropayment rails | **N** | Y (Base) | N | P (x402 Foundation member) | N | N | N | N |

Sources: [Stripe stablecoins](https://docs.stripe.com/payments/stablecoin-payments) · [Bridge Open Issuance](https://stripe.com/blog/introducing-open-issuance-from-bridge) · [Checkout and Fireblocks](https://checkout.com/newsroom/checkout-com-scales-stablecoin-settlement-for-us-merchants-in-partnership-with-fireblocks) · [Checkout Blue EMI](https://checkout.com/newsroom/checkout-com-acquires-blue-emi-a-licensed-issuer-of-euro-stablecoins-and-establishes-lithuania-technology-centre)

**Stripe and Checkout.com are building here; Adyen and the entire India stack are not.** No Indian competitor has a crypto or stablecoin product. This is the least actionable column in the document for an India-focused platform, and is listed for completeness rather than as a candidate.

---

## S. Data, reporting and warehouse integration

| | PayGate | Stripe | Razorpay | Adyen | Checkout.com | Cashfree | PayU IN | Juspay |
|---|---|---|---|---|---|---|---|---|
| SQL access to your own data | **P (direct Postgres, not a product)** | **Y (Sigma)** | N | N | N | N | N | P (ClickHouse in Decision Engine) |
| Warehouse export (Snowflake/BigQuery/S3) | **N** | **Y (Data Pipeline, 3 h loads)** | N | **N (SFTP only)** | N | N | N | N |
| Reporting API (async report runs) | **P (`/v1/reports`)** | Y | P | P (webhook + GET, or SFTP) | Y (`/reports`) | Y | P | N |
| Granular accounting-entry API | **Y (ledger)** | Y | N | N | **Y (`/financial-actions`)** | N | N | Y (recon add-on) |
| Scheduled report delivery | **P** | Y | Y | Y | Y | Y | P | N |
| ERP connectors (NetSuite/SAP/QuickBooks) | **N** | Y (NetSuite paid; QB/Xero apps) | P (Zoho/Tally) | Y (NetSuite, Salesforce OMS, Oracle) | N | P (Zoho) | P | N |
| Managed database over your data | **N** | P (Stripe Database, preview) | N | N | N | N | N | N |
| Anomaly detection on ops metrics | **P (Prometheus rules)** | N | N | N | N | N | **Y (Overwatch, ML + rules)** | **Y (Outage Alerts + audit anomalies)** |

Sources: [Stripe Sigma](https://docs.stripe.com/sigma) · [Stripe Data Pipeline](https://docs.stripe.com/data/access-data-in-warehouse) · [Adyen reporting](https://docs.adyen.com/reporting/automatically-get-reports) · [Checkout financial-actions](https://api-reference.checkout.com/) · [PayU Overwatch](https://docs.payu.in/docs/payu-monitoring-alerts-overwatch)

Adyen — the largest acquirer in this field — has **no warehouse export at all**, only SFTP with company-wide scope. Stripe stands alone with a real data product.

---

## T. Regulatory and licensing gates (India)

This section is why several rows above read **N** and will keep reading **N**. It is documented, not inferred.

| Authorization | What it gates | Razorpay | Cashfree | PayU | Juspay |
|---|---|---|---|---|---|
| **PA-O** (online aggregation) | gateway, links, pages, QR, split settlement, holding merchant funds | final Dec 2023 | 19.12.2023 | **May 2025** | 6 Feb 2024 |
| **PA-P** (offline / physical) | POS, SoftPOS, soundbox acquiring | **22 Jan 2026** | 19.12.2023 | Nov 2025 | — |
| **PA-CB** (cross-border) | import / export collections, LRS flows | Dec 2025 | Jul 2024 (E&I) | Nov 2025 | Nov 2025 |
| **NBFC** | lending on own book | **rejected** | none | **held (PayU Finance, 2018)** | none |
| **PPI** | wallets, gift cards, prepaid | unconfirmed | 25.10.2024 | held | — |
| **TPAP + PSP-bank sponsorship** | anything on UPI rails | Axis, Airtel Payments Bank | sponsor bank | sponsor bank | Yes Bank |
| **Token Requestor (CoFT)** | card-on-file tokenization | Y | Y | Y | **Y — TR *and* TSP** |
| **EMVCo 3DS** | running a 3DS server or ACS | – | – | Y (Wibmo) | Y |

Net-worth requirement for a PA: **₹15 Cr at application, ₹25 Cr by 31 Mar 2026**, plus an escrow account with a scheduled commercial bank and Tp+0/Tp+1 remittance discipline. [RBI PA/PG circular](https://www.rbi.org.in/Scripts/NotificationUser.aspx?Id=11822) · [PA-CB circular](https://www.rbi.org.in/Scripts/NotificationUser.aspx?Id=12561) · [RBI authorised-entity list](https://www.rbi.org.in/Scripts/PublicationsView.aspx?id=12043)

**The embargo precedent matters more than the licence itself.** The RBI barred *both* Razorpay and Cashfree from onboarding new merchants for roughly a year (reported as ~Dec 2022 to Dec 2023), and returned PayU's application in Jan 2023 with an order to restructure — pausing its new online merchant onboarding for about two years. [embargo lift](https://yourstory.com/2023/12/rbi-lifts-embargo-for-razorpay-cashfree-to-operate-as-payment-aggregators) · [PayU](https://www.businesstoday.in/latest/corporate/story/rbi-asks-payu-to-re-apply-for-payment-aggregator-licence-company-pauses-new-merchant-on-boarding-359865-2023-01-11)

**What is *not* licence-gated**, and is therefore actually available:

1. **Orchestration and routing** — as long as funds never touch you. This is Juspay's entire history.
2. **Verification APIs** — Cashfree's Secure ID is a standalone revenue line with no PA requirement. Note the gate here is different in kind: government-ID lookups need **data-source access agreements** (NSDL, UIDAI, DigiLocker, GSTN), not an RBI authorization. Bank-account and VPA verification ride payment rails and need a sponsor relationship.
3. **Vaulting and portability** — subject to PCI DSS, which is an audit programme, not an authorization.
4. **Billing, invoicing, tax, revenue recognition, reporting** — software over someone else's rails.
5. **Risk, fraud scoring, dispute tooling, reconciliation** — pure data products.
6. **Developer surface** — SDKs, CLI, MCP, sandboxes, documentation, published operational contracts.

Everything that **settles money, holds balances, issues credit, issues cards, or runs on UPI rails** is licence- or sponsor-bank-gated. That line is the most important single fact in this document for prioritisation, and it is why every entry in `FEATURES.md` carries its gating.

---

## Evidence and limits

* Competitor rows are **documented** — each traceable to the vendor's own documentation, newsroom, or an RBI/NPCI primary source, linked inline.
* PayGate rows are **read from this repository or measured**, not asserted.
* Research hit this session's 200-call web-search cap partway through; later findings were filled by direct page fetches. Where a vendor's *absence* of a product is claimed, read it as **"not present on that vendor's own surfaces"** rather than proven absent. This specifically affects: Adyen's lack of a stablecoin product, Adyen's lack of an issuer-processing business sold to third-party banks, and Cashfree's lack of any lending entity.
* Vendor marketing sometimes outruns vendor documentation. Where Stripe's Sessions-2026 recap and its docs disagree — Managed Payments GA, stablecoin-backed cards GA, 3DS Standalone GA — this document follows the docs.
* Checkout.com's `/docs/*` pages are JavaScript-rendered and returned title-only on direct fetch, so some Checkout.com detail comes from search snippets rather than the doc page itself.
* Two India items remain unconfirmed: Razorpay's PPI authorization status, and the exact date the Razorpay/Cashfree onboarding embargo was *imposed* (the lift date is corroborated by the RBI list).

---

## Where PayGate genuinely leads

Not marketing claims — each verified in this repository or by measurement:

1. **Double-entry ledger exposed as a first-class concept**, balanced to zero across every transaction under live traffic. No competitor exposes this to merchants.
2. **Idempotency semantics** stronger than Razorpay's or PayU's — verified: 8 parallel identical requests create exactly one order; key reuse with a different body returns `409`.
3. **Three-way reconciliation** with a mismatch workflow, comparable to Adyen's and ahead of PayU's.
4. **Merchant-configurable velocity rules**, which Stripe, Adyen and Checkout.com only offer partially.
5. **Transactional outbox** as an explicit architectural commitment — the *design* is right even though the *implementation* currently drains at 1 event/sec.
6. **Cross-tenant isolation** verified by adversarial testing rather than asserted.

Part II adds two more, both of which only became visible once the comparison widened:

7. **Self-hostable with a readable core.** Only Juspay/Hyperswitch shares this property, and it is the reason Hyperswitch has 43k GitHub stars. Every other comparator is a closed SaaS.
8. **A block-and-debit mandate primitive already in the domain model** (`UPIMandate` with `AmountLimit` and `RetryWindowHours`). This is structurally the same object as Stripe's Shared Payment Token and India's UPI Reserve Pay — the agentic-commerce primitive the whole field is racing to ship.

## Where the gaps are structural, not incidental

* **No real acquiring rails.** Every method is simulator-backed. This is a category difference, not a feature gap, and it bounds any comparison above.
* **No PCI DSS certification.** The design avoids raw PAN, which is the hard part, but certification is an audit and a compliance programme.
* **No published operational contract** — no rate limits, no retry schedule, no delivery guarantee, no SLA, no status page. Competitors compete on these documents as much as on code, and PayGate's underlying behaviour is often already good enough to publish.
* **No licence, and no path to one.** ₹25 Cr net worth by 31 Mar 2026, an escrow account with a scheduled commercial bank, and an authorization that the RBI has demonstrably suspended or returned for every major incumbent. This closes issuing, banking, lending, POS acquiring and direct UPI participation permanently, not temporarily.
* **No named optimisation product.** Section O is the single densest area of competitor investment and PayGate's emptiest row. Unlike the licensed categories, most of it — decline-conditioned retries, outcome-feedback routing, cost observability — is unblocked logic over data already being recorded.
* **No AI/agent surface at all.** Every one of the seven comparators ships an MCP server. This is a wrapper over an existing REST API, not a capability gap.

## What the wider comparison changes

The narrow read (Part I) said PayGate needed better acceptance parity: partial capture, network tokens, SCA exemptions. Those remain true but are the *least* strategically interesting conclusions, because they are the places where every incumbent is strongest and several are licence-gated.

The wide read (Part II) says something different. The categories where the field is thin **and** the work is unblocked are:

1. **Revenue recognition and accrual reporting** — only Stripe has it, and PayGate's double-entry ledger is already the correct substrate. This is the clearest case in the document of an existing strength being one product away from a differentiator.
2. **Orchestration as a product** — the one large category that provably does not require a payment-aggregator licence, and the shape `internal/gateway` already has.
3. **The published operational contract** — rate limits, retry schedule, delivery guarantee, latency percentiles. Nobody in this field publishes latency percentiles; PayGate already measures them.
4. **Agent-facing surface** — an MCP server plus scoped payment grants built on the mandate primitive that already exists.
5. **Verification APIs** — data-access-gated rather than licence-gated, already started (`internal/upiverify`), and a proven standalone revenue line for Cashfree.

`FEATURES.md` enumerates all of this — every feature any competitor has, regardless of merit, ordered by priority and marked with its gating.
