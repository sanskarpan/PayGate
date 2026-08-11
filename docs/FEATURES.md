# PayGate — Feature Catalogue

Every feature any comparator ships, whether or not it is a good idea for PayGate. Compiled 2026-08-11 from `COMPETITIVE-ANALYSIS.md`, which carries the source links for each vendor claim.

The instruction behind this document was deliberate: **if anyone has it, it is listed here.** Merit is not a filter. Several entries are things PayGate should never build; they are catalogued anyway, in the lowest tier, so that the map has no blank regions.

---

## How to read this

**Who ships it** — `S` Stripe · `R` Razorpay · `A` Adyen · `C` Checkout.com · `CF` Cashfree · `P` PayU India (incl. Wibmo, LazyPay) · `J` Juspay (incl. Hyperswitch). Lowercase means partial or preview.

**PayGate** — `Have` (verified in this repo) · `Partial` (exists but incomplete or nominal) · `None`.

**Gate** — what stands between PayGate and shipping it:

| Code | Meaning |
|---|---|
| `—` | Nothing. Pure software over data already held. |
| `PCI` | PCI DSS scope — an audit programme, not an authorization. |
| `DATA` | Data-source access agreements (NSDL, UIDAI, DigiLocker, GSTN, bureaus). |
| `SCHEME` | Card-network certification (EMVCo 3DS, Token Requestor/TSP, Click to Pay, Visa Direct). |
| `PA-O` / `PA-P` / `PA-CB` | RBI Payment Aggregator authorization — online / offline / cross-border. |
| `NBFC` / `PPI` | RBI entity licences — lending on own book / prepaid instruments. |
| `BANK` | A partner-bank relationship. Non-banks cannot hold deposits. |
| `TPAP` | NPCI sponsor-bank or TPAP arrangement. Nothing touches UPI rails directly. |
| `BIN` | BIN sponsorship plus card-scheme membership (issuing). |
| `HW` | Hardware design, certification and supply chain. |
| `RAIL` | A real acquiring connection. PayGate's methods are simulator-backed. |

Priority runs **P0 → P6**. P0–P3 are unblocked. P4–P5 are gated. P6 is catalogued for completeness only.

---

# P0 — Contract and correctness gaps in what PayGate already claims

These are not new products. They are places where PayGate's *existing* surface is incomplete, undocumented, or nominally present but non-functional. Every one is unblocked, and each is cheaper than anything below it.

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Partial capture | S A C CF j | **None** | — | `internal/payment/postgres.go:820` rejects `amount != current.Amount`. Razorpay also lacks it; everyone else has it. |
| Multi-capture (several captures per auth) | s a C CF J | **None** | — | Checkout.com caps at 150, Stripe at 50+1 on IC+. |
| Overcapture / capture above auth | s J | **None** | — | Hyperswitch exposes it explicitly. |
| Published rate limits | S C CF J | **None** | — | Effective limit is 10 rps per key per path and is undocumented. |
| Published webhook retry schedule | S A C CF J | **None** | — | Adyen 9 s→8 h over 30 days; Juspay 16 tries / 24 h. |
| Published delivery guarantee | S R C CF J | **None** | — | PayGate is at-least-once and unordered in fact; it says so nowhere. |
| Published idempotency TTL | S C | **None** | — | The implementation is stronger than Razorpay's or PayU's. It is simply unstated. |
| Dated / versioned API | S A CF P J | **None** | — | `/v1` path only, no version header, no diff tool. |
| Cursor pagination everywhere | S A C J | **Partial** | — | Cursor on orders, offset elsewhere. |
| Public status page | S R A CF J | **None** | — | Checkout.com's is login-gated; PayU has none. Low bar to clear. |
| Published uptime SLA | A j | **None** | — | Only Adyen publishes a contractual figure (99.9% quarterly). |
| Published latency percentiles | *nobody* | **None** | — | Already measured in `PERFORMANCE-BASELINE.md`. Nobody in the field publishes these. |
| Authenticated merchant signup | S R A C CF P J | **None** | — | `POST /v1/merchants` is unauthenticated. Every comparator gates this. |
| Dashboard write parity with the API | S R A C CF P J | **Partial** | — | Webhooks, settlements, recon, mandates and capture/refund are read-only in the UI. |
| Bounded-budget risk decisioning with declarative fallback | S A C | **Partial** | — | Stripe allows 2 s, Adyen 2000 ms, both fall back to stored rules. PayGate evaluates inline with no stated budget. |
| Outbox drain throughput | — | **Partial** | — | Measured at 1.00 events/sec; webhook lag p50 224 s. The design is right, the implementation is not. |

---

# P1 — Unblocked differentiators

Categories where the field is thin, the work needs no licence, and PayGate has an existing asset to build on.

## Ledger-native accounting

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Double-entry ledger exposed to merchants | j | **Have** | — | Only Hyperswitch's paid Recon module comes close. |
| Revenue recognition (accrual, ASC 606) | S | **None** | — | **Only Stripe has this.** The ledger is already the right substrate. |
| Per-second / daily revenue amortization | S | **None** | — | |
| Trial balance, income statement, balance sheet | S | **None** | — | |
| Revenue waterfall report | S | **None** | — | |
| AR ageing report | S | **None** | — | |
| Chart-of-accounts mapping | S | **None** | — | |
| Period close and reopen | S | **None** | — | |
| Custom revenue rules (defer, exclude, allocate, treat-as-tax) | S | **None** | — | |
| Custom fiscal calendars (4-5-4) | s | **None** | — | |
| External (off-platform) revenue data import | S | **None** | — | |
| Granular accounting-entry API | S C j | **Have** | — | Checkout.com's `/financial-actions` is the closest analogue. |
| Balance-account abstraction over the ledger | S A | **Partial** | — | Ledger has 7 accounts; there is no per-merchant balance-account object. |
| Sweep rules (push/pull by schedule or threshold) | S A | **None** | — | Pure ledger logic until money actually moves. |
| Fund allocation across multiple ledgers | CF | **None** | — | Cashfree's Co-Lending three-ledger split. |

## Orchestration as a product

Per the licensing research, this is the one large category that provably does not require a PA authorization, provided merchant funds are never touched. Juspay reached $1.2B as a technology service provider before taking a licence in Feb 2024.

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Multi-PSP routing on one integration | R CF P J s | **Partial** | — | `internal/gateway` router exists; it routes to simulators. |
| Rule-based routing (method, BIN, issuer, amount, region) | R A C CF P J | **Partial** | — | Razorpay's Optimizer rules on card IIN, brand, issuer, bank, amount. |
| Success-rate dynamic ranking from outcome feedback | R A C CF P J | **None** | — | Hyperswitch's Decision Engine is open source and readable. |
| Cost-aware / least-cost routing | A C CF P J | **None** | — | Juspay does multi-objective re-ranking. |
| Volume-split / percentage routing | J | **None** | — | |
| A/B testing of routing on live traffic | A C J r | **None** | — | Adyen runs 50/50 current-vs-recommended. |
| Autopilot self-tuning routing | J | **None** | — | |
| PSP downtime detection | CF P J | **Partial** | — | Circuit breaker exists; there is no detection *product*. |
| Automatic cascade on outage | CF P J | **Partial** | — | |
| Decline-code-conditioned retry policy | S A C J | **Partial** | — | Failover on error only. Adyen's Auto Rescue is ML-timed. |
| Connector SDK / bring-your-own-PSP | J | **Partial** | — | Internal interface, not a published extension point. |
| Unified cross-PSP reporting | R CF P J | **None** | — | |
| Cross-processor retry | s J | **None** | — | |
| Migration tooling from a rival PSP | CF J | **None** | — | Cashfree ships Razorpay/Juspay/PayU migration guides as agent skills. |

## Agent-facing surface

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| MCP server | S R C CF P J a | **None** | — | **All seven comparators ship one.** A wrapper over the existing REST API. |
| Hosted MCP with OAuth (no API keys) | S C | **None** | — | Checkout.com's is the most mature; Adyen's is a local alpha. |
| Scoped single-use payment grant | S A C CF P | **Partial** | — | `UPIMandate` has `AmountLimit` and `RetryWindowHours` — structurally the same object. |
| Agent spend controls and approval flows | s | **None** | — | |
| `llms.txt` / machine-readable docs index | R CF J | **None** | — | |
| Agent skills manifest | S CF | **None** | — | |
| CLI | S R P | **None** | — | |
| Agent-facing product catalogue / feed | S A | **None** | — | Adyen's Agentic Feed + Cart. |
| Machine-to-machine micropayments | S | **None** | — | Stripe MPP; card leg min $0.50. |
| In-dashboard agentic execution | S r p | **None** | — | |
| ACP / UCP / AP2 / x402 protocol support | S A c | **None** | — | Adyen supports UCP + AP2 + ACP; Checkout.com ACP only. |
| n8n / LangChain / Dify integrations | R CF | **None** | — | |

## Verification as a product

`internal/upiverify` already ships one of the 19 APIs Cashfree sells as a standalone revenue line. Note these are gated by **data-access agreements**, not by an RBI authorization.

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| UPI VPA verification | R CF P J | **Have** | TPAP | Already shipped. |
| Bank account verification (penny drop) | S R CF P | **None** | BANK | |
| Reverse penny drop | CF | **None** | BANK | Verify without debiting. |
| IFSC verification | CF | **None** | — | Static dataset; genuinely unblocked. |
| PAN verification | CF P | **None** | DATA | |
| Aadhaar verification / masking | CF P | **None** | DATA | |
| DigiLocker document fetch | CF R P | **None** | DATA | |
| Aadhaar eSign | CF P | **None** | DATA | |
| GSTIN / GSTIN-with-PAN | CF | **None** | DATA | |
| CIN / Udyam (KYB) | CF | **None** | DATA | |
| Voter ID, driving licence, passport, vehicle RC | CF | **None** | DATA | |
| Face match | C CF p | **None** | — | Model-side; no external gate. |
| Face liveness | C CF | **None** | — | |
| Name match | CF | **None** | — | Pure string/semantic matching. |
| Smart OCR on documents | CF | **None** | — | |
| Video KYC (AI, multilingual) | C CF P | **None** | DATA | |
| Hosted KYC links | C CF | **None** | — | |
| Geocoding / address verification | CF | **None** | DATA | |
| Mobile 360 (phone intelligence) | CF | **None** | DATA | |
| Employment verification | CF | **None** | DATA | |
| Document verification (global, 3000+ types) | S C | **None** | DATA | |
| AML / sanctions / PEP / adverse media screening | C a | **Partial** | DATA | Currently simulated. |
| One-click onboarding SDK | CF | **None** | — | |

## The published operational contract

Beyond the P0 items, competitors compete on documents as much as on code.

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Documented failure modes and recovery | — | **Have** | — | `FAILURE-MODES.md`, `RUNBOOK.md`, `DR-PLAN.md` already exist and exceed the field. |
| Published SLOs | — | **Have** | — | `OBSERVABILITY-SLOS.md`. |
| Published performance methodology | *nobody* | **Have** | — | `PERFORMANCE-BASELINE.md`. Genuinely unmatched here. |
| Sandbox with test credentials | S R A C CF J | **Have** | — | |
| Isolated sandboxes with external collaborators | S | **None** | — | |
| Claimable sandbox API for partners | S | **None** | — | |
| Event schema registry with rollouts | — | **Have** | — | No comparator exposes this. |
| Webhook event catalogue | S R A C CF J | **Have** | — | |
| Signed webhooks (HMAC + RFC 9421) | S R A C CF J | **Have** | — | |
| Audit trail | C j | **Have** | — | Stronger than Stripe's 180-day retention. |
| Anomaly detection on operational metrics | P J | **Partial** | — | Prometheus rules exist; PayU's Overwatch is an ML product with criticality scoring. |

---

# P2 — Table stakes the whole field has and PayGate does not

Unblocked, unglamorous, and conspicuous by absence.

## Billing and subscriptions

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Subscription lifecycle | S R CF P j | **Have** | — | Fixed amount, day/week/month only. |
| Price / plan object separate from subscription | S | **None** | — | Prerequisite for everything below it. |
| Quantity and per-seat pricing | S | **None** | — | |
| Tiered pricing (graduated) | S | **None** | — | |
| Volume pricing | S | **None** | — | |
| Package / unit-block pricing | S | **None** | — | |
| Multi-currency price options | S | **None** | — | |
| Proration on plan change | S | **None** | — | |
| Subscription schedules with phases | S | **None** | — | Stripe supports 10 phases. |
| Pending / scheduled updates | S | **None** | — | |
| Pause and resume | S | **None** | — | |
| Cancel at period end | S | **None** | — | |
| Trials | S r | **None** | — | |
| Trial reminder emails | S | **None** | — | |
| Usage-based / metered billing | S | **None** | — | Stripe now routes this through Metronome. |
| Meter events API with grace window | S | **None** | — | |
| Entitlements (feature access derived from plan) | S | **None** | — | |
| Credit grants / prepaid balances | S p | **None** | — | Append-only ledger — natural fit here. |
| Coupons and promotion codes | S R CF P J | **None** | — | |
| Stackable discounts with restrictions | S | **None** | — | |
| Offers engine (no-code, method-conditional) | R CF P J | **None** | — | Every India competitor has one. |
| No-cost EMI offer construction | R CF P J | **None** | — | |
| Dunning / retry on failed renewal | S A J | **Partial** | — | Fixed `RetryIntervalHours`; Stripe times retries with a model. |
| Dunning email sequences | S | **None** | — | |
| Hosted recovery page | S | **None** | — | |
| Self-serve customer portal | S r | **None** | — | |
| Cancellation deflection survey | S | **None** | — | |
| Billing automations (trigger → action) | S | **None** | — | |
| Billing analytics (MRR, churn, LTV, cohorts) | S p j | **None** | — | |
| Mandate import from another provider | CF | **None** | — | |
| eNACH / physical NACH mandates | R CF P | **None** | BANK | |
| Card-on-file recurring (MIT) | S R A C CF P J | **Partial** | RAIL | |

## Invoicing

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Invoice object with lifecycle | S R P | **Have** | — | Records exist. |
| Invoice PDF rendering | S R P | **None** | — | |
| GST-compliant invoicing | R P | **None** | — | India table stakes. |
| E-invoicing (statutory formats) | S | **None** | DATA | |
| Credit notes | S | **None** | — | |
| Customer credit balance | S | **None** | — | |
| Invoice editing after issue | S | **None** | — | |
| Unapply / reapply payments | S | **None** | — | |
| Payment reminders | S R | **Partial** | — | `MarkInvoiceReminded` exists; no delivery. |
| Custom invoice templates | S | **None** | — | |
| Quotes with revisions | S | **None** | — | |
| Pricing table web component | S | **None** | — | |

## Checkout and conversion surfaces

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Hosted checkout | S R A C CF P J | **Have** | — | |
| Embedded / drop-in component | S R A C CF P J | **Partial** | — | Sandbox page only. |
| Per-method UI components | S A C CF J | **None** | — | Cashfree's Element SDK. |
| Payment links | S R A C CF P J | **Have** | — | |
| Payment pages / no-code storefront | R CF P | **None** | — | |
| Payment buttons | R CF P | **None** | — | |
| Payment forms | CF | **None** | — | |
| QR codes (static and dynamic) | R CF P | **Have** | RAIL | UPI QR exists as a simulated rail. |
| One-click returning-customer checkout | S R C CF P J | **None** | — | Razorpay Magic, Checkout.com Flow Remember Me. |
| Address prefill from a shared network | R CF P | **None** | DATA | Razorpay cites ~100M users. |
| Address serviceability logic | R | **None** | — | |
| Saved payment methods / QuickPay | S R C CF J | **Partial** | — | Card vault exists; there is no checkout surface for it. |
| CVV-less repeat payment | R CF J | **None** | SCHEME | |
| Auto-OTP read | R CF P J | **None** | — | |
| Native OTP on merchant page | CF P J | **None** | RAIL | PayU cites 15+ banks. |
| Payment-method recommendation / ranking | P J | **None** | — | |
| A/B testing of payment-method mix | J s | **None** | — | |
| Abandoned-checkout recovery | CF | **None** | — | |
| WhatsApp cart recovery | CF | **None** | — | |
| WhatsApp native payments | CF P | **None** | RAIL | |
| Retry-on-failure inside the checkout | R J | **None** | — | |
| Multi-language checkout | S A C | **None** | — | Checkout.com Flow supports 20+ incl. RTL. |
| Trust badge / buyer-protection widget | R | **None** | — | |
| Affordability widget on the product page | R P | **None** | — | |
| Checkout theming without code | R J | **Partial** | — | |
| Checkout analytics (funnel, abandonment) | R J | **None** | — | |

## Developer surface

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Server SDKs, 6+ languages | S R A C CF | **Partial** | — | Two (Go, JS). |
| Mobile SDKs (iOS, Android) | S R A C CF P J | **None** | — | |
| React Native / Flutter SDKs | CF P J s | **None** | — | |
| E-commerce plugins (Shopify, Woo, Magento) | S R A C CF P J | **None** | — | |
| Webhook debugging / replay UI | S | **Partial** | — | |
| API request log inspector | S | **None** | — | |
| Error explorer | S | **None** | — | |
| Hosted API explorer / shell | S | **None** | — | |
| App marketplace | S | **None** | — | |
| Partner / referral programme APIs | R CF P | **None** | — | |
| Co-branded OAuth onboarding | CF P | **None** | — | |
| Data migration in and out | S J | **None** | — | |

## Reporting and data

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Async report runs | S C CF | **Partial** | — | `/v1/reports` exists. |
| Scheduled report delivery | S R A C CF | **Partial** | — | |
| CSV / XLS exports | S R A C CF P J | **Have** | — | |
| SQL access to your own data | S | **Partial** | — | Direct Postgres, not a product surface. |
| Warehouse export (Snowflake, BigQuery, S3) | S | **None** | — | Adyen has none of this either. |
| SFTP report delivery | A | **None** | — | |
| ERP connectors (NetSuite, SAP, QuickBooks, Zoho, Tally) | S R A cf p | **None** | — | |
| Settlement reconciliation report | S R A C CF P J | **Have** | — | Three-way, with a mismatch workflow. |
| Fee / interchange breakdown report | S A C | **None** | RAIL | |
| Tax reporting exports | S | **None** | — | |
| Location / jurisdiction reports | s | **None** | — | |

---

# P3 — Competitive depth

Unblocked, but each is a substantial build rather than a gap-fill.

## Authorization-rate optimisation

The densest area of competitor investment in the whole comparison.

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Named, branded optimisation product | S A C J | **None** | — | Everyone has one; PayGate has a circuit breaker. |
| Issuer-preference message reformatting | S A C J | **None** | RAIL | |
| Silent retry on soft decline | S A C J | **None** | RAIL | |
| Excessive-retry prevention | S | **None** | — | Pure counter logic. |
| Retry timing model | S A J | **None** | — | |
| Recovered-volume / lift reporting | S A C | **None** | — | |
| Cost-savings reporting | s J | **None** | RAIL | |
| Interchange downgrade detection | J | **None** | RAIL | Juspay flags >7-day shipment delay, AVS mismatch. |
| Scheme auth-penalty tracking | J | **None** | RAIL | |
| Invoice / fee-markup integrity audit vs contract | J | **None** | — | |
| Cross-border FX and regulatory cost tracking | J | **None** | — | |
| Cost-anomaly root-causing | J | **None** | — | |

## Risk and fraud

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Rules engine with merchant-editable rules | S A C CF J | **Have** | — | Stripe disables this in India. |
| Velocity rules | CF | **Have** | — | Ahead of most of the field. |
| Numeric risk score exposed | S C CF | **Partial** | — | |
| ML fraud model | S R A C CF P J | **None** | — | |
| Cross-merchant network signals | S A C | **None** | — | Requires scale PayGate does not have. |
| Device fingerprinting | C CF J | **None** | — | Checkout.com's Risk.js returns 0–100. |
| Bot / card-testing detection | S A | **None** | — | |
| Free-trial and multi-account abuse signals | s | **None** | — | |
| Cross-session shopper graph | A | **None** | — | Adyen's ShopperDNA. |
| Review queue / case management | S A C | **Partial** | — | Risk events exist; there is no queue UI. |
| Allow / block lists | S A C CF | **Have** | — | |
| Rule backtesting | S C | **None** | — | |
| Shadow testing against live traffic | C | **None** | — | |
| Explainability per transaction | C | **Partial** | — | |
| Dynamic risk thresholds | s | **None** | — | |
| Merchant risk rating | CF | **Partial** | — | Cashfree publishes AAA–BBB. |
| Platform-side account/merchant risk scoring | S | **None** | — | |
| Dynamic reserves driven by risk | S | **Partial** | — | Reserve policy exists; it is not risk-driven. |
| Payout-side risk screening | CF | **None** | — | Cashfree Payout Protect. |
| RTO / COD fraud prediction | R CF | **None** | DATA | India-specific and genuinely valuable. |
| COD control (nudge-to-prepay, differential fee, disable) | R CF | **None** | — | |
| Card-scheme monitoring (VAMP, ECP/EFM) thresholds | A | **None** | RAIL | |

## Disputes

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Dispute API with evidence submission | S R A C CF J | **Have** | — | |
| Accept-dispute endpoint | S R A C CF | **Have** | — | PayU lacks one. |
| Evidence library / templates | S | **None** | — | |
| AI-assembled evidence | S | **None** | — | |
| Automatic defense without merchant action | A | **None** | — | |
| Pre-dispute deflection (RDR, Order Insight) | C | **None** | SCHEME | |
| Dispute analytics | S C J | **Partial** | — | |
| Chargeback protection / liability transfer | s | **None** | — | Missing from Stripe's current plan comparison. |

## Reconciliation

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Two-way reconciliation | S R A C CF P J | **Have** | — | |
| Three-way (orders / PSP / bank) | A J | **Have** | — | |
| Mismatch workflow | A J | **Have** | — | |
| Connector-based ingestion from PSPs, banks, ERPs | J | **None** | — | |
| Exception categorisation and assignment | J | **None** | — | |
| Auto-resolution on rerun | J | **None** | — | Juspay claims 80%. |
| Multi-region / multi-currency recon | J | **None** | — | |
| Per-transaction settlement detail | A | **Partial** | — | |

## Tokenization and vault

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Own card vault | S R A C CF P J | **Have** | PCI | |
| Card fingerprint / cross-session join key | S A | **Have** | — | `fingerprintHash` exists and is unused as a join key. |
| CVV tokenisation as a separate primitive | C | **None** | PCI | |
| Pay-then-vault / vault-then-pay | J | **None** | PCI | |
| Bring-your-own-vault deployment | J CF P | **None** | PCI | Hyperswitch ships 7 deployment models. |
| Forward vaulted credentials to third parties | C J s a p | **None** | PCI | Checkout.com's `/forward` is the cleanest version. |
| PAN import from a prior processor | S J | **None** | PCI | |
| PAN export / portability out | J | **None** | PCI | Nobody offers self-serve export. Differentiating if done. |
| Token groups shared across merchants | A | **None** | — | |

## Settlement and payouts

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Scheduled settlement | S R A C CF P | **Have** | RAIL | |
| Settlement hold / release | S A C | **Have** | — | |
| Settlement statements | S R A C CF P | **Have** | — | |
| On-demand / instant settlement | S R A C CF P | **None** | BANK | |
| Configurable settlement cycles | C CF | **None** | — | |
| Payout beneficiary management | R CF P J | **Have** | — | |
| Payout approval workflow / maker-checker | A CF | **Have** | — | |
| Bulk / batch payouts | R CF P J | **Have** | — | |
| Payout links (no bank details needed) | R CF P | **None** | BANK | Cashgram, Payout Links. |
| Pay-to-phone-number | CF | **None** | TPAP | |
| Payout recall and reversal | CF | **None** | BANK | |
| Scheduled payouts | J | **None** | — | |
| Payout smart retries | J | **None** | — | |
| Multi-source fund management with low-balance queuing | CF | **None** | BANK | |
| Instant refunds | R CF P | **None** | RAIL | |
| Marketplace split at authorization | A C CF P | **Partial** | — | Split instructions exist. |
| Split at capture and at refund | A C CF | **Partial** | — | |
| Rule-based automatic splits | A | **None** | — | |
| Fixed / variable / compound commission models | C | **None** | — | |
| Vendor onboarding and vendor dashboard | CF | **Partial** | — | |
| Deferred vendor settlement | CF | **None** | — | |
| Multi pay-in (allocate third-party-acquired funds) | A | **None** | — | Distinctive to Adyen. |

## Platform and merchant lifecycle

| Feature | Who ships it | PayGate | Gate | Note |
|---|---|---|---|---|
| Sub-merchant / connected accounts | S R A C CF P J | **Have** | — | |
| Capabilities model | S R A J | **Have** | — | |
| Reserves (fixed and rolling) | C s r a | **Have** | — | |
| KYC / KYB onboarding API | S R A C CF P | **Partial** | DATA | Screening is simulated. |
| Hosted onboarding pages | S A C CF P | **None** | — | Adyen's links are single-use, 4-minute. |
| Embedded onboarding components | S A C | **None** | — | |
| Networked onboarding (reuse across platforms) | S | **None** | — | |
| Legal-entity / business-line model | A | **Partial** | — | |
| Verification deadlines with remediation windows | A | **None** | — | Adyen uses 30/60-day. |
| Embedded components library (payouts, disputes, reports) | S | **None** | — | Stripe ships ~24. |
| Platform-configurable pricing / commissions | S C CF | **Partial** | — | |
| Responsibility model (fees vs losses collector) | S | **None** | — | |
| White-label / embedded payments for platforms | CF P | **None** | — | |
| RBAC with granular permissions | S R A C CF J | **Have** | — | |
| Team invitations and roles | S R A C CF | **Have** | — | |
| Data retention policies | — | **Have** | — | No comparator exposes this. |

---

# P4 — Gated by licence, partner or certification

Everything here requires something PayGate cannot obtain by writing code. Listed in full because the *design* is often transferable even when the product is not — see `COMPETITIVE-ANALYSIS.md` §T for why these stay closed.

## Payment methods and rails

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Real card acquiring | S R A C CF P J | **None** | `PA-O` `RAIL` |
| Real UPI acquiring | R CF P J | **None** | `TPAP` |
| UPI collect | S R CF J p | **None** | `TPAP` |
| UPI Lite | J r | **None** | `TPAP` |
| RuPay credit card on UPI | R CF P J | **None** | `TPAP` |
| Credit line on UPI | R CF P | **None** | `TPAP` `NBFC` |
| Prepaid wallets on UPI | R | **None** | `TPAP` `PPI` |
| UPI Reserve Pay / one-time mandate | R CF P | **None** | `TPAP` |
| UPI Circle | r | **None** | `TPAP` |
| In-app UPI without app switch (plugin SDK) | R P J | **None** | `TPAP` |
| Own UPI switch | R J | **None** | `TPAP` `BANK` |
| White-label TPAP platform | R J | **None** | `TPAP` |
| UPI-in-a-box for banks (issuer/acquirer switch) | J | **None** | `TPAP` `BANK` |
| Netbanking (real bank connections) | R CF P J a c | **None** | `PA-O` `BANK` |
| Banking Connect / NBBL netbanking intent | CF | **None** | `BANK` |
| Wallets (Paytm, PhonePe, Amazon Pay) | R CF P J | **None** | `PA-O` |
| EMI (credit card, debit card, cardless, no-cost) | R CF P J | **None** | `PA-O` `NBFC` |
| BNPL aggregation | R A C CF P J | **None** | `NBFC` |
| International cards | S R A C CF P J | **None** | `PA-CB` |
| Apple Pay / Google Pay | S A C cf p | **None** | `SCHEME` |
| Click to Pay | J s | **None** | `SCHEME` |
| SEPA / ACH / Bacs direct debit | S A C | **None** | `BANK` |
| Real-time payment rails (RTP, FPS, PIX) | S A C J | **None** | `BANK` |
| Third-party validation (TPV) | R CF P J | **None** | `TPAP` |
| BBPS biller / agent | CF P | **None** | `BANK` |
| Sodexo / meal-card acceptance | R | **None** | `PA-O` |

## Issuing

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Virtual card issuing | S A C P | **None** | `BIN` |
| Physical card issuing | S A C | **None** | `BIN` `HW` |
| Metal / custom card bundles | S | **None** | `BIN` `HW` |
| Spend controls (MCC, velocity, limits) | S A C | **None** | `BIN` |
| Merchant-ID-level controls | s | **None** | `BIN` |
| Real-time authorization decisioning webhook | S A C | **None** | `BIN` |
| Rule-based fallback on webhook timeout | S A | **None** | `BIN` |
| Lifecycle controls (single-use auto-cancel) | S | **None** | `BIN` |
| 3DS on issued cards | S A C P | **None** | `BIN` `SCHEME` |
| Digital wallet provisioning (manual + push) | S A C P | **None** | `BIN` `SCHEME` |
| PAN and PIN reveal SDK | S A C P | **None** | `BIN` `PCI` |
| PIN change (AES / ISO-4 PIN block) | A | **None** | `BIN` |
| Issuing disputes | S A | **None** | `BIN` |
| Cardholder KYC flows | S A C | **None** | `BIN` `DATA` |
| Issuing balance funding | S A | **None** | `BIN` `BANK` |
| Consumer credit issuing with bureau reporting | s p | **None** | `BIN` `NBFC` |
| Cards issued to AI agents | s | **None** | `BIN` |
| Prepaid card platform | P | **None** | `PPI` |
| Issuer instalments | P | **None** | `NBFC` |
| Issuer ACS (3DS on the issuing side) | P | **None** | `SCHEME` `BANK` |
| Cross-border issuing programmes | s | **None** | `BIN` |

## In-person and unified commerce

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Terminal hardware | S R A P | **None** | `PA-P` `HW` |
| Terminal API (nexo / server-driven) | S R A CF | **None** | `PA-P` |
| Local vs cloud terminal connectivity | A | **None** | `PA-P` |
| SoftPOS / phone-as-terminal | CF | **None** | `PA-P` |
| Tap to Pay on iPhone | S A | **None** | `PA-P` `SCHEME` |
| Tap to Pay on Android | S A CF | **None** | `PA-P` `SCHEME` |
| Standalone / no-code terminal mode | A s | **None** | `PA-P` |
| Apps-on-devices (run your app on the reader) | S A | **None** | `PA-P` `HW` |
| Offline EMV / store-and-forward | S A | **None** | `PA-P` |
| Terminal fleet management API | S A | **None** | `PA-P` |
| OTA terminal updates | S A | **None** | `PA-P` |
| Hardware orders API | S A | **None** | `PA-P` `HW` |
| Soundbox / audio payment confirmation | R | **None** | `PA-P` `HW` |
| Unattended terminals | A | **None** | `PA-P` `HW` |
| Autonomous stores (entry pre-auth, exit capture) | A | **None** | `PA-P` |
| EMV-compliant receipts | S A | **None** | `PA-P` |
| Incremental / extended authorization | S A | **None** | `RAIL` |
| Dynamic currency conversion at the terminal | R P | **None** | `PA-P` |
| US debit AID selection / least-cost routing | A s | **None** | `SCHEME` |
| Cross-channel shopper identity | A | **None** | `RAIL` |
| Buy-online-return-in-store (referenced refunds) | A | **None** | `PA-P` |
| Endless aisle | A | **None** | `PA-P` |
| Payment-linked loyalty (card as loyalty ID) | A | **None** | `RAIL` |
| Agent-mode offline collection | CF | **None** | `PA-P` |

## Banking, treasury and embedded finance

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Business current accounts | S R a | **None** | `BANK` |
| Financial accounts for platforms | S A | **None** | `BANK` |
| Real account details (IBAN, sort code) | S A | **None** | `BANK` |
| Virtual accounts for collections | R CF a | **Have** | `BANK` (simulated today) |
| Virtual UPI IDs for collections | R CF | **None** | `TPAP` |
| Escrow accounts | R CF | **None** | `BANK` |
| Sweeps and auto top-up | S A | **None** | `BANK` |
| Outgoing transfers by priority (instant/wire/cross-border) | S A | **None** | `BANK` |
| Direct debit acceptance into the account | A | **None** | `BANK` |
| Vendor payments / AP automation | R | **None** | `BANK` |
| Automatic TDS on vendor payments | R | **None** | `BANK` `DATA` |
| Payroll with statutory filing (TDS/PF/ESI/PT) | R | **None** | `BANK` `DATA` |
| Tax payments | R | **None** | `BANK` `DATA` |
| Corporate credit cards | S R A C P | **None** | `BIN` `BANK` |
| Source-to-pay / procurement | R | **None** | `BANK` |
| Merchant wallet (closed/semi-closed/open loop) | P | **None** | `PPI` |
| Loyalty points as a wallet balance | R | **None** | `PPI` |
| Gift cards | R | **None** | `PPI` |
| Instant payouts to sellers (monetizable) | S | **None** | `BANK` |
| Multi-currency settlement | S A C CF | **None** | `PA-CB` `BANK` |
| Instant currency conversion | S | **None** | `PA-CB` |
| FX rate locks / held rates | C s | **None** | `PA-CB` |
| FX quotes API | s | **None** | `PA-CB` |
| Cross-border export collections | S R A C CF P J | **None** | `PA-CB` |
| Cross-border import / LRS flows | R CF P | **None** | `PA-CB` |
| Global payouts to non-customers | S J | **None** | `PA-CB` |
| Outward remittance | CF | **None** | `PA-CB` |
| FIRA / statutory export documentation | CF | **None** | `PA-CB` `DATA` |
| Merchant of record (Stripe holds the contract) | S | **None** | `PA-O` |
| Adaptive presentment pricing | S | **None** | `PA-CB` |

## Capital and lending

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Merchant cash advance | S R A P | **None** | `NBFC` |
| Revenue-based repayment from processing volume | S R A | **None** | `NBFC` |
| Line of credit | R P s | **None** | `NBFC` |
| Settlement acceleration as a credit product | R CF P | **None** | `NBFC` `BANK` |
| Grant-offer pre-assessment API | A | **None** | `NBFC` |
| Capital-for-platforms (embed lending) | s R | **None** | `NBFC` |
| Consumer BNPL on own book | P | **None** | `NBFC` |
| Personal loans | P | **None** | `NBFC` |
| Multi-lender credit routing at checkout | R CF P J | **None** | `NBFC` |
| LOS / LMS for NBFCs (DLG-compliant) | R | **None** | `NBFC` `DATA` |
| Co-lending disbursal with escrow split | CF | **None** | `BANK` |
| Underwriting from processing history | S R A | **Partial** | — | *The input already exists in the ledger; only disbursal is gated.* |

## Authentication and network services

| Feature | Who ships it | PayGate | Gate |
|---|---|---|---|
| Real 3DS2 challenge | S R A C CF P J | **None** | `SCHEME` `RAIL` |
| 3DS decision manager / adaptive 3DS | S C P J a | **None** | `SCHEME` |
| SCA exemption engine (TRA, low-value, MIT, data-only) | S A C J | **None** | `SCHEME` |
| Standalone / acquirer-agnostic 3DS server | C P J a s | **None** | `SCHEME` |
| Data-only 3DS for scheme-fee reduction | S A | **None** | `SCHEME` |
| Delegated / biometric authentication | C P | **None** | `SCHEME` |
| Passkey authentication | P J | **None** | `SCHEME` |
| OTP-free flows | P | **None** | `SCHEME` |
| Risk-based authentication | P | **None** | `SCHEME` |
| Network tokenization with cryptogram | S R A C CF P J | **Partial** | `SCHEME` — *field only, no cryptogram* |
| Token requestor certification | R CF P J | **None** | `SCHEME` |
| Token service provider (issue tokens for others) | J | **None** | `SCHEME` |
| Bring-your-own token requestor ID | CF J P | **None** | `SCHEME` |
| Co-badged debit network tokens | A | **None** | `SCHEME` |
| Real-time account updater | S A C J | **None** | `SCHEME` |
| Push provisioning to wallets | C P | **None** | `SCHEME` |
| PINless debit routing | S A C | **None** | `SCHEME` |
| Card metadata / BIN lookup API | C | **None** | `SCHEME` |
| Card payouts (Visa Direct, Mastercard Send) | CF J | **None** | `SCHEME` |

---

# P5 — Catalogued for completeness

Real products at real competitors. None of them is a sensible target for PayGate; all are listed because the instruction was to omit nothing.

| Feature | Who ships it | Gate | Why it is here and not above |
|---|---|---|---|
| Stablecoin acceptance | S c | `PA-CB` | No Indian competitor has any crypto product. |
| Stablecoin settlement to merchant wallets | C S | `PA-CB` | Checkout.com uses Fireblocks; US-only, state-restricted. |
| Stablecoin payouts | S | `PA-CB` | |
| Stablecoin-denominated financial accounts | S | `BANK` | |
| Issuing your own stablecoin | S C | `BANK` | Checkout.com acquired a Lithuanian EMI to do it. |
| Operating a settlement blockchain | S | — | Stripe's Tempo. |
| Crypto on-ramp (fiat → crypto) | S | `PA-CB` | Stripe is merchant of record. |
| Stablecoin-backed issued cards | s | `BIN` | |
| x402 / on-chain agent micropayments | S a | — | |
| Custodial and non-custodial wallet infrastructure | S | `BANK` | Stripe's Privy acquisition. |
| DeFi yield on balances | S | `BANK` | |
| Company incorporation (Delaware C-corp / LLC) | S | — | Stripe Atlas. |
| Automatic 83(b) filing | S | — | |
| SAFE creation and cap-table tracking | S | — | |
| Startup incorporation programme (India) | R | — | Razorpay Rize. |
| Carbon-removal purchasing | S | — | Stripe Climate. |
| Carbon offset resale to your own customers | S | — | |
| Founder perks marketplace | S | — | |
| Insurance distribution | P | — | LazyPay. |
| Mobile recharge / utility top-up | P | `BANK` | |
| Bill payment as a consumer service | P | `BANK` | |
| Retail loyalty programme management | R | — | Razorpay Engage. |
| Loyalty points as a payment method | P | `PPI` | PayU Loyalty Edge. |
| Bank/brand rewards partner integration | P | `SCHEME` | |
| Marketing data feed to ad platforms | a | `DATA` | Adyen's Data Connect is closed to new accounts. |
| ONDC network participation | r | `PA-O` | Secondary sources only. |
| Foreign-market payment gateway (Malaysia) | R | foreign licence | Razorpay Curlec, BNM-registered. |
| White-label processing for other PSPs | P | `PA-O` | Wibmo PG. |
| Issuer processing sold to banks | P J | `BANK` | |
| Core UPI components built for NPCI | J | `TPAP` | |
| ATM / branch switch software | *retired* | `BANK` | Wibmo's legacy line, now discontinued. |

---

## Sequencing

The tiers above are ordered by leverage, not by effort. Read together with `GAP-ANALYSIS.md`, the practical sequence is:

1. **Close P0.** None of it is new product work. It is finishing and documenting what already exists — partial capture, the operational contract, authenticated signup, dashboard write parity, and the outbox drain rate. Every claim in Part I of the competitive matrix that reads worse than it should reads that way because of something in P0.

2. **Pick one P1 line and go deep.** The three with the best ratio of differentiation to effort:
   - **Revenue recognition on the existing ledger** — the only genuinely uncontested category in the entire comparison outside Stripe.
   - **Orchestration as a published product** — the one large area that provably needs no licence, and the shape `internal/gateway` already has.
   - **MCP server plus scoped payment grants** — every comparator ships the former; PayGate already has the domain object for the latter.

3. **Fill P2 selectively.** These are table stakes, but a portfolio project does not need all of them. Prioritise by whether the absence *undermines a claim PayGate already makes* — a price object matters because subscriptions exist; mobile SDKs matter much less.

4. **Treat P4 as design exercises, not roadmap.** The valuable transfer from issuing is the bounded-budget authorization webhook. From unified commerce it is cross-channel identity. From treasury it is sweep rules over balance accounts. Each of those is buildable; the licensed product around it is not.

5. **P5 exists to be complete.** Nothing in it should be built.

---

## Provenance

Competitor claims trace to `COMPETITIVE-ANALYSIS.md`, which carries a source link for each. PayGate status is read from this repository at commit `3c70966` or measured in `PERFORMANCE-BASELINE.md` — not asserted. Where a vendor is marked as lacking something, that means it is absent from the vendor's own documentation and product pages, which is weaker than proof of absence; the research hit this session's web-search cap and finished on direct page fetches.
