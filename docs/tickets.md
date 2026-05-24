# PayGate Expansion Backlog

This document captures the next-wave product, platform, compliance, and operator tickets that could be added to PayGate beyond the current implemented scope.

It is intentionally written as a backlog rather than a roadmap. The point is to preserve detail, dependencies, and acceptance criteria so these items can be turned into GitHub issues, PRDs, or sprint tasks without rediscovering the reasoning.

## How To Use This Document

- Each ticket is written to be issue-ready.
- Tickets are grouped by capability area rather than team.
- Priority is relative:
  - `P0`: foundational or compliance-critical
  - `P1`: high-value product/platform expansion
  - `P2`: meaningful but not immediate
  - `P3`: polish or later-scale work
- Status defaults to `proposed` unless explicitly changed.

## Planning Principles

- Preserve the current money-core strength: Postgres-backed order -> payment -> ledger -> settlement correctness should stay the system’s consistency boundary.
- Avoid fake breadth. New methods should be productized end to end, not just added as new enum values.
- Prefer capability gating over permissive launches. Merchant activation, payouts, and high-risk methods should not be enabled by default.
- Keep runtime, docs, tests, and dashboard surfaces aligned. A feature does not count as complete if it exists only in schema, only in docs, or only in tests.
- Treat India-first payment realism as a competitive priority: UPI, mandates, merchant onboarding, payout verification, and reporting matter more than cosmetic protocol breadth.

## External Reference Baselines

- NPCI UPI overview: <https://www.npci.org.in/what-we-do/upi/product-overview/>
- NPCI UPI AutoPay overview: <https://www.npci.org.in/what-we-do/autopay/product-overview>
- RBI payment aggregator guidelines: <https://www.rbi.org.in/scripts/RTGS_Notification.aspx?Id=11822>
- RBI KYC master direction index: <https://systemhealth.rbi.org.in/Scripts/BS_ViewMasDirections.aspx_id%3D11566%281%29.html>
- Razorpay UPI docs: <https://razorpay.com/docs/payments/payment-methods/upi/>
- Stripe Connect baseline: <https://docs.stripe.com/connect/how-connect-works>
- Stripe 3DS baseline: <https://docs.stripe.com/payments/3d-secure/standalone-three-d-secure>
- Stripe reporting baseline: <https://docs.stripe.com/reports>
- Stripe risk baseline: <https://docs.stripe.com/radar/risk-evaluation?locale=en-GB>
- PCI DSS 4.0 summary of changes: <https://listings.pcisecuritystandards.org/documents/PCI-DSS-v3-2-1-to-v4-0-Summary-of-Changes-r1.pdf>
- HTTP Message Signatures: <https://www.ietf.org/rfc/rfc9421.html>

---

## Epic A: India-First Payment Method Expansion

### PGX-001: Productize UPI Intent Flow

- Priority: `P0`
- Status: `proposed`
- Why:
  - Current code recognizes `upi` as a method, but there is no real UPI intent lifecycle.
  - This is the highest-value missing payment capability for an India-first platform.
- Scope:
  - create UPI intent payment initiation API
  - generate deep-link payloads and app-handoff metadata
  - persist UPI-specific payment metadata and state transitions
  - support completion via callback or active status polling
  - expose status in dashboard and webhook payloads
- Acceptance criteria:
  - merchant can create a UPI intent payment and receive actionable client payload
  - payment moves through `created -> pending_customer_action -> processing -> captured/failed`
  - callback replays and duplicate notifications are idempotent
  - UPI-specific reason codes are normalized into the payment domain
  - integration tests cover timeout, duplicate callback, late success, and customer abandon
- Dependencies:
  - PGX-024
  - PGX-041

### PGX-002: Productize UPI QR Flow

- Priority: `P0`
- Status: `proposed`
- Why:
  - Merchant acceptance in India often depends on QR-first flows, especially for mobile and in-person style experiences.
- Scope:
  - generate static and dynamic QR payloads
  - store QR session lifecycle and expiry
  - link scan-completion callbacks to orders/payments
  - support merchant-facing expiry, cancellation, and reuse rules
- Acceptance criteria:
  - dynamic QR can be created with amount and expiry
  - expired QR cannot settle a payment
  - late callbacks after expiry are handled deterministically
  - dashboard can show pending, expired, and completed QR sessions
- Dependencies:
  - PGX-001

### PGX-003: Add UPI VPA Validation And Payee Verification

- Priority: `P1`
- Status: `proposed`
- Why:
  - UPI requires quality controls around payee identifiers before more advanced payout and collect flows are added.
- Scope:
  - validate VPA syntax
  - support optional verification callout to upstream provider
  - persist verification evidence and freshness
  - expose verification status for payout destination setup
- Acceptance criteria:
  - invalid VPAs are rejected early
  - verification records are versioned and timestamped
  - payout/mandate flows can require fresh verification

### PGX-004: Implement UPI Autopay Mandate Lifecycle

- Priority: `P0`
- Status: `proposed`
- Why:
  - Recurring mandates are a major gap for subscriptions and billing products.
- Scope:
  - mandate create, activate, pause, revoke, expire
  - initial customer approval flow
  - schedule metadata and retry window support
  - mandate-linked payment attempts and audit trail
- Acceptance criteria:
  - active mandates can trigger recurring collections
  - revoked or expired mandates cannot be charged
  - mandate changes emit webhooks and audit logs
  - dashboard exposes mandate state and history
- Dependencies:
  - PGX-001
  - PGX-028

### PGX-005: Add Netbanking As A Real Redirect Method

- Priority: `P1`
- Status: `proposed`
- Why:
  - Netbanking currently exists only as a configured method label, not a real payment product.
- Scope:
  - bank selection
  - redirect initiation
  - callback and polling completion
  - bank-specific failure normalization
- Acceptance criteria:
  - merchant can initiate bank redirect payments
  - return/cancel/error flows are normalized into core payment states
  - tests cover missing return, duplicate callback, and provider timeout

### PGX-006: Add Wallet Payment Lifecycle

- Priority: `P2`
- Status: `proposed`
- Why:
  - Wallet support exists in enums but not as a productized flow.
- Scope:
  - wallet method configuration
  - provider redirect or token-based confirmation
  - wallet-specific status and error mapping
- Acceptance criteria:
  - wallet flows are distinct from card and UPI, not generic reuse
  - dashboard and reporting can break down wallet volume separately

### PGX-007: Build Payment Link Product

- Priority: `P1`
- Status: `proposed`
- Why:
  - Payment links are a standard product surface for merchants without deep integration.
- Scope:
  - hosted link create/update/expire
  - order generation on first visit or pre-linked order strategy
  - branding, expiry, reminders, partial restrictions
- Acceptance criteria:
  - merchants can create and share payment links
  - links can expire or be manually disabled
  - link payments are traceable back to the link entity in reporting

### PGX-008: Build Invoice And Collect Request Surface

- Priority: `P2`
- Status: `proposed`
- Why:
  - Invoices and collect requests are natural merchant-facing monetization primitives.
- Scope:
  - invoice entity
  - due date and reminder lifecycle
  - payment link or UPI/QR attachment
  - settlement/reporting connection
- Acceptance criteria:
  - invoice status derives from real payment state
  - overdue and partial cases are visible to merchants

---

## Epic B: Card Stack And Processor Abstraction

### PGX-009: Create Tokenization Service Boundary

- Priority: `P0`
- Status: `proposed`
- Why:
  - Current system lacks a real vault/CDE boundary for card data handling.
- Scope:
  - dedicated tokenization service or module boundary
  - opaque token issuance
  - one-time and reusable token classes
  - token metadata and network references
- Acceptance criteria:
  - card PAN never needs to transit the main app after tokenization
  - payment auth can operate on tokens only
  - token access is fully audited
- Dependencies:
  - PGX-010
  - PGX-042

### PGX-010: Add Saved Card And Customer Vault Support

- Priority: `P1`
- Status: `proposed`
- Why:
  - Saved cards are table stakes for repeat merchants and subscriptions.
- Scope:
  - customer entity or customer reference layer
  - merchant-scoped card token storage
  - default payment method management
  - token lifecycle controls
- Acceptance criteria:
  - merchant can tokenize and reuse cards safely
  - customer/card ownership is enforced
  - saved cards can be disabled or deleted

### PGX-011: Implement 3DS / Challenge Flow Support

- Priority: `P0`
- Status: `proposed`
- Why:
  - Real card acceptance requires authentication flows separate from authorization success paths.
- Scope:
  - `requires_action` payment state
  - challenge session handoff
  - authentication result persistence
  - timeout and abandonment handling
- Acceptance criteria:
  - payment may pause awaiting customer action
  - challenge completion can resume the payment safely
  - late challenge completion after timeout does not corrupt state
- Dependencies:
  - PGX-009

### PGX-012: Add Processor Routing Layer

- Priority: `P1`
- Status: `proposed`
- Why:
  - Current `GatewayClient` shape assumes a single processor.
- Scope:
  - processor capability registry
  - merchant routing policy
  - failover and override support
  - success-rate and cost-based routing inputs
- Acceptance criteria:
  - auth requests can route to more than one provider path
  - routing decision is persisted and inspectable
  - retries do not accidentally double-authorize across processors
- Dependencies:
  - PGX-009
  - PGX-011

### PGX-013: Add Network Token And Card Metadata Normalization

- Priority: `P2`
- Status: `proposed`
- Why:
  - Mature processors distinguish PAN tokens, network tokens, funding type, issuer, BIN, and card country.
- Scope:
  - card metadata model
  - normalized issuer/network/funding fields
  - token type awareness
- Acceptance criteria:
  - risk and routing systems can use normalized card metadata

---

## Epic C: Merchant Onboarding, KYB, And Compliance

### PGX-014: Create Merchant KYB Application Model

- Priority: `P0`
- Status: `proposed`
- Why:
  - Merchant onboarding is currently account creation, not regulated onboarding.
- Scope:
  - legal entity profile
  - business classification
  - registration/tax identifiers
  - onboarding state machine
- Acceptance criteria:
  - merchant cannot be treated as fully active without onboarding completion
  - application lifecycle is auditable

### PGX-015: Add Beneficial Owner And Controller Records

- Priority: `P0`
- Status: `proposed`
- Why:
  - KYB without ownership/control records is incomplete.
- Scope:
  - beneficial owner model
  - controller/authorized signatory model
  - ownership percentage and verification evidence
- Acceptance criteria:
  - onboarding review requires minimum owner/controller completeness
  - changes are versioned and auditable

### PGX-016: Add Document Intake And Verification Workflow

- Priority: `P0`
- Status: `proposed`
- Why:
  - Compliance onboarding needs document storage, review, and verification states.
- Scope:
  - document upload metadata
  - storage integration
  - document review queue
  - expiry and re-request flows
- Acceptance criteria:
  - documents can be requested, uploaded, reviewed, approved, or rejected
  - expired documents re-open the merchant review state
- Dependencies:
  - PGX-014

### PGX-017: Add Sanctions / PEP / AML Screening Hooks

- Priority: `P0`
- Status: `proposed`
- Why:
  - Compliance review requires external screening and evidence retention.
- Scope:
  - screening request/response model
  - case status
  - positive-match review process
- Acceptance criteria:
  - onboarding cannot finalize without completed screening
  - screening outcomes are auditable and re-runnable

### PGX-018: Add Merchant Capability Gating

- Priority: `P0`
- Status: `proposed`
- Why:
  - Method and payout availability should depend on compliance/risk status.
- Scope:
  - capability flags for payments, payouts, refunds, high-risk methods
  - automatic restriction and re-enable logic
- Acceptance criteria:
  - merchants can be `restricted` without deleting configuration
  - gateway enforces capability state, not just dashboard visibility
- Dependencies:
  - PGX-014
  - PGX-017

### PGX-019: Add Reserve, Rolling Reserve, And Settlement Hold Policies

- Priority: `P1`
- Status: `proposed`
- Why:
  - Real platforms need merchant-specific reserve and delayed settlement controls.
- Scope:
  - reserve policy model
  - rolling reserve calculation
  - release schedule
  - operator override
- Acceptance criteria:
  - settlements and payouts reflect reserve deductions accurately
  - policy changes are audit logged and backtestable

---

## Epic D: Payouts, Settlements, And Treasury-Like Controls

### PGX-020: Add Beneficiary / Payee Entity

- Priority: `P0`
- Status: `proposed`
- Why:
  - Current payouts do not model a reusable payout destination or beneficiary lifecycle.
- Scope:
  - beneficiary record
  - bank account and VPA destination types
  - verification and approval status
- Acceptance criteria:
  - payouts must target an approved beneficiary
  - beneficiary changes are tracked historically

### PGX-021: Add Bank Account Verification And Penny-Drop Support

- Priority: `P1`
- Status: `proposed`
- Why:
  - Payout rails need verified bank destinations before scaling usage.
- Scope:
  - account verification request/response
  - verification freshness
  - failure and retry handling
- Acceptance criteria:
  - unverified accounts can be blocked or limited by policy
  - payout setup retains verification evidence
- Dependencies:
  - PGX-020

### PGX-022: Add Payout Approval Workflow

- Priority: `P1`
- Status: `proposed`
- Why:
  - High-value payouts often require maker-checker or threshold approvals.
- Scope:
  - approval policy engine
  - multi-actor approval timeline
  - emergency override
- Acceptance criteria:
  - approval thresholds are configurable
  - unauthorized execution attempts are blocked and audited

### PGX-023: Add Bulk Payouts And Batch Controls

- Priority: `P2`
- Status: `proposed`
- Why:
  - Many merchant operations depend on batch disbursement capability.
- Scope:
  - payout batch import and validation
  - dry-run and preview
  - partial failure handling
- Acceptance criteria:
  - batch processing is idempotent
  - merchant and operator can see item-level results

### PGX-024: Add Merchant Settlement Preferences And Schedules

- Priority: `P1`
- Status: `proposed`
- Why:
  - Settlements should be configurable by merchant and method.
- Scope:
  - daily/weekly/manual schedules
  - payout minimum thresholds
  - holiday/weekend rules
- Acceptance criteria:
  - settlement creation respects merchant schedule
  - changes do not retroactively corrupt historical batches

### PGX-025: Add Settlement Statement Generation

- Priority: `P1`
- Status: `proposed`
- Why:
  - Current system can compute settlements but does not provide finance-grade merchant statements.
- Scope:
  - PDF/CSV exports
  - line-item detail
  - fee/tax breakdown
- Acceptance criteria:
  - merchants can download settlement statements for any closed period
  - totals tie to ledger-backed settlement data

### PGX-026: Add Refund Reversals / Settlement Re-open Handling

- Priority: `P1`
- Status: `proposed`
- Why:
  - Cross-period refunds and returns create treasury and reporting edge cases.
- Scope:
  - post-settlement refund adjustments
  - reopened or offset settlements
  - accounting-safe reversal markers
- Acceptance criteria:
  - post-settlement refund effects are explicit and traceable
  - finance operators can reconcile offsets cleanly

---

## Epic E: Subscriptions, Billing, And Platform Monetization

### PGX-027: Add Customer Entity And Billing Profile

- Priority: `P1`
- Status: `proposed`
- Why:
  - Recurring billing and saved methods need a customer-level abstraction.
- Scope:
  - customer lifecycle
  - contact info
  - payment preferences
  - merchant-scoped identity model
- Acceptance criteria:
  - subscriptions and saved cards can anchor to customer records

### PGX-028: Add Subscription And Recurring Billing Model

- Priority: `P0`
- Status: `proposed`
- Why:
  - Recurring revenue is a major missing payments product.
- Scope:
  - plan/subscription/invoice attempt entities
  - billing schedules
  - renewal retry rules
  - pause/cancel/proration behavior
- Acceptance criteria:
  - subscription billing attempts produce normal payment records
  - mandate/card method reuse works safely
- Dependencies:
  - PGX-004
  - PGX-010
  - PGX-027

### PGX-029: Add Smart Collect / Virtual Account Support

- Priority: `P2`
- Status: `proposed`
- Why:
  - B2B and enterprise collection flows often rely on virtual account rails rather than interactive checkout.
- Scope:
  - virtual account issuance
  - inbound collection matching
  - remitter tracking
- Acceptance criteria:
  - inbound payments can be auto-matched to merchants/orders
  - unmatched deposits enter a review queue

### PGX-030: Add Marketplace / Split Payment Model

- Priority: `P1`
- Status: `proposed`
- Why:
  - Platform businesses need split payouts, fees, and sub-merchant relationships.
- Scope:
  - split instructions
  - platform fee model
  - sub-merchant/account linkage
- Acceptance criteria:
  - payment ledgering can represent platform fee + merchant receivable split safely
  - settlement and payout reporting show split beneficiaries
- Dependencies:
  - PGX-012
  - PGX-020

---

## Epic F: Risk, Fraud, And Trust Controls

### PGX-031: Add Device / Browser Fingerprinting Inputs

- Priority: `P1`
- Status: `proposed`
- Why:
  - Current risk signals are too shallow for fraud-heavy payment environments.
- Scope:
  - device fingerprint ingestion
  - normalized session and browser attributes
  - risk feature persistence
- Acceptance criteria:
  - risk evaluations can include device-based signals
  - sensitive raw identifiers are handled with clear retention rules

### PGX-032: Add BIN, Country, Issuer, And Network Risk Inputs

- Priority: `P1`
- Status: `proposed`
- Why:
  - Card risk depends on issuer/network/geographic mismatch analysis.
- Scope:
  - BIN enrichment
  - issuer country and funding type
  - mismatch and sanctions heuristics
- Acceptance criteria:
  - risk events can explain issuer/network-driven outcomes

### PGX-033: Add Review Queue For Manual Risk Decisions

- Priority: `P1`
- Status: `proposed`
- Why:
  - Risk systems need a human escalation path for ambiguous cases.
- Scope:
  - review queue model
  - review decisions and notes
  - SLA and assignment fields
- Acceptance criteria:
  - `risk_hold` payments can be approved or blocked manually
  - queue actions emit audit logs and webhooks if needed

### PGX-034: Add Merchant-Level Fraud Configuration

- Priority: `P2`
- Status: `proposed`
- Why:
  - Merchants vary by risk appetite and business model.
- Scope:
  - threshold configuration
  - allowlists and blocklists
  - review thresholds by merchant
- Acceptance criteria:
  - merchant config changes affect new evaluations only
  - defaults remain centrally governed

### PGX-035: Add Reserve Escalation From Risk Signals

- Priority: `P2`
- Status: `proposed`
- Why:
  - Fraud and dispute patterns should influence reserve posture, not just block individual payments.
- Scope:
  - risk-to-reserve policy triggers
  - reserve review timeline
  - operator approval path
- Acceptance criteria:
  - reserve changes can be traced back to measurable events

---

## Epic G: Reporting, Reconciliation, And Finance Operations

### PGX-036: Add Merchant Download Center

- Priority: `P1`
- Status: `proposed`
- Why:
  - Operators and merchants need downloadable operational and finance data, not only UI views.
- Scope:
  - CSV export queue
  - signed URL access
  - report catalog
- Acceptance criteria:
  - merchants can request and download large exports asynchronously

### PGX-037: Add Payment / Refund / Dispute / Payout Statements

- Priority: `P1`
- Status: `proposed`
- Why:
  - Payments products need finance-grade statements by period and entity type.
- Scope:
  - standard period statements
  - fee and tax sections
  - downloadable and API access
- Acceptance criteria:
  - statement totals reconcile with ledger-backed aggregates

### PGX-038: Add GST / Tax Reporting Workflow

- Priority: `P2`
- Status: `proposed`
- Why:
  - Indian merchant operations often require explicit tax reporting support.
- Scope:
  - tax component storage
  - report formats
  - merchant tax settings
- Acceptance criteria:
  - taxes are represented consistently across invoices, statements, and exports

### PGX-039: Add Advanced Reconciliation Sources

- Priority: `P1`
- Status: `proposed`
- Why:
  - Reconciliation currently centers on internal and simulated external consistency, but real systems need processor/bank/rail source imports.
- Scope:
  - source file/API ingestion
  - reconciliation adapters
  - source-specific mismatch categories
- Acceptance criteria:
  - recon can compare internal state against at least one external settlement source

### PGX-040: Add Recon Operator Workflow And Resolution Queue

- Priority: `P1`
- Status: `proposed`
- Why:
  - Detecting mismatches is only half the value; finance teams need a structured resolution process.
- Scope:
  - assignment and status model
  - notes, attachments, and resolution codes
  - reopen and escalation flows
- Acceptance criteria:
  - every mismatch can move through a managed resolution lifecycle

---

## Epic H: Platform Surface, Runtime Alignment, And Architecture

### PGX-041: Wire Saga APIs Into Main Gateway Or Remove Them From Public Contracts

- Priority: `P0`
- Status: `proposed`
- Why:
  - The main gateway, integration mux, and docs are not currently aligned on advanced platform APIs.
- Scope:
  - mount saga routes in main gateway or explicitly remove them from public contract docs
  - align auth, docs, and tests
- Acceptance criteria:
  - runtime, OpenAPI, and integration tests advertise the same public surface

### PGX-042: Wire Event Schema APIs Into Main Gateway Or Remove Them From Public Contracts

- Priority: `P0`
- Status: `proposed`
- Why:
  - Schema governance exists in code and tests but is not consistently exposed in runtime.
- Scope:
  - same alignment pattern as PGX-041
- Acceptance criteria:
  - no drift between runtime and documented availability

### PGX-043: Wire Ledger Hold APIs Into Main Gateway Or Remove Them From Public Contracts

- Priority: `P0`
- Status: `proposed`
- Why:
  - Ledger hold controls should not be half-public.
- Scope:
  - same alignment pattern as PGX-041
- Acceptance criteria:
  - hold operations are either truly supported or clearly internal-only

### PGX-044: Replace Empty Service Stubs With Real Extraction Plan Or Remove Them

- Priority: `P1`
- Status: `proposed`
- Why:
  - `cmd/order-service`, `cmd/payment-service`, `cmd/refund-service`, and `cmd/recon-worker` are placeholders today.
- Scope:
  - either implement real bootstraps and boundaries
  - or remove stubs and keep extraction documented only
- Acceptance criteria:
  - repo no longer implies runtime services that do not actually exist

### PGX-045: Add Internal Command Envelope Standard

- Priority: `P2`
- Status: `proposed`
- Why:
  - As orchestration grows, internal command metadata should be standardized across services.
- Scope:
  - command ID
  - causation/correlation IDs
  - retry metadata
  - ack/nack semantics
- Acceptance criteria:
  - async orchestration commands share one contract and one observability model

### PGX-046: Add Method-Specific State Models

- Priority: `P1`
- Status: `proposed`
- Why:
  - A single generic payment state machine becomes hard to reason about once cards, UPI, mandates, netbanking, and wallets all diverge.
- Scope:
  - shared core payment state + method-specific substates or specialized models
- Acceptance criteria:
  - method-specific flows do not overload generic states ambiguously

---

## Epic I: Security, Compliance, And Data Protection

### PGX-047: Add Secret / KMS / Key Hierarchy Strategy

- Priority: `P0`
- Status: `proposed`
- Why:
  - A production payment platform needs a real secrets and encryption design, not just environment variables.
- Scope:
  - KMS-backed secrets plan
  - encryption key hierarchy
  - rotation policy
- Acceptance criteria:
  - all critical secrets have defined storage, rotation, and access boundaries

### PGX-048: Add Field-Level Encryption For Sensitive Merchant And Payout Data

- Priority: `P1`
- Status: `proposed`
- Why:
  - Bank accounts, legal IDs, and compliance data should not sit as plain application data.
- Scope:
  - encrypted columns or application-layer encryption
  - key versioning
  - migration strategy
- Acceptance criteria:
  - sensitive fields are protected at rest with measurable access controls

### PGX-049: Upgrade Webhook Signing To HTTP Message Signatures

- Priority: `P2`
- Status: `proposed`
- Why:
  - Current HMAC-style signing is useful, but the platform can offer a more modern, structured signing mode.
- Scope:
  - optional RFC 9421-compatible signing
  - transition path for existing integrations
- Acceptance criteria:
  - merchants can opt into stronger webhook verification semantics

### PGX-050: Add Artifact Retention And Data Lifecycle Policies

- Priority: `P1`
- Status: `proposed`
- Why:
  - Compliance data, audit data, and operational exports all require explicit retention policies.
- Scope:
  - retention rules by artifact class
  - purge/archive workflows
  - legal hold support
- Acceptance criteria:
  - retention policies are enforced and documented

---

## Epic J: Dashboard, Merchant Experience, And Operator UX

### PGX-051: Add Merchant Onboarding Review Console

- Priority: `P1`
- Status: `proposed`
- Why:
  - Compliance onboarding needs a first-class operator workflow, not ad hoc data inspection.
- Scope:
  - review queue
  - document panel
  - capability decision actions
- Acceptance criteria:
  - operator can move merchant from pending review to restricted or active from one surface

### PGX-052: Add Risk Review Queue UI

- Priority: `P1`
- Status: `proposed`
- Why:
  - Risk holds need manual resolution tooling.
- Scope:
  - queue, notes, approve, block, escalation
- Acceptance criteria:
  - risk operators can process held payments without API-only workflows

### PGX-053: Add Payout Approval UI

- Priority: `P1`
- Status: `proposed`
- Why:
  - Maker-checker payout controls need a dashboard surface.
- Scope:
  - approval inbox
  - threshold indicators
  - action audit
- Acceptance criteria:
  - approval workflows are fully operable in UI

### PGX-054: Add Report Download Center UI

- Priority: `P1`
- Status: `proposed`
- Why:
  - Exports need a discoverable and manageable UI surface.
- Scope:
  - report request screen
  - download history
  - failure/retry state
- Acceptance criteria:
  - merchants and operators can request and retrieve exports without API-only flows

### PGX-055: Add Advanced Filters, Saved Views, And Bulk Actions

- Priority: `P2`
- Status: `proposed`
- Why:
  - As volume grows, list pages need operator-grade navigation.
- Scope:
  - saved filters
  - bulk state actions where safe
  - portfolio and team views
- Acceptance criteria:
  - high-volume operators can work without manually re-entering filters

### PGX-056: Add Saga / Schema / Ledger Hold Console

- Priority: `P2`
- Status: `proposed`
- Why:
  - Advanced platform controls should not be API-only if they are treated as public/operator features.
- Scope:
  - dedicated pages for orchestration, schema rollout, and hold state inspection
- Acceptance criteria:
  - operator can inspect and act on these advanced systems from the dashboard

---

## Epic K: Observability, Reliability, And Runtime Operations

### PGX-057: Add Low-Cardinality HTTP Route Metrics

- Priority: `P1`
- Status: `proposed`
- Why:
  - Current HTTP metrics label by raw URL path, which will not scale well for high-cardinality IDs.
- Scope:
  - route-template metrics labeling
  - deprecation of raw-path labels
- Acceptance criteria:
  - metrics remain useful at scale without cardinality blow-up

### PGX-058: Add Processor And Method-Specific SLO Dashboards

- Priority: `P2`
- Status: `proposed`
- Why:
  - Mature payment operations need approval rate, latency, and failure-rate breakdowns by method and provider.
- Scope:
  - SLO definitions
  - dashboards
  - alerts
- Acceptance criteria:
  - operator can see auth success and latency by payment method and provider

### PGX-059: Add Post-Restore Automated Validation Suite

- Priority: `P1`
- Status: `proposed`
- Why:
  - DR runbooks exist, but automatic restore validation should be hardened.
- Scope:
  - restore smoke automation
  - reconciliation validation
  - idempotency/outbox replay checks
- Acceptance criteria:
  - restore success is validated by repeatable automated checks, not only manual signoff

### PGX-060: Add Chaos Profiles For Async And Settlement Components

- Priority: `P2`
- Status: `proposed`
- Why:
  - Existing chaos coverage can go deeper into payout/settlement/webhook edge cases.
- Scope:
  - rail callback disorder
  - processor timeout storms
  - outbox lag and consumer restart scenarios
- Acceptance criteria:
  - chaos suite covers the highest-risk async money-path dependencies

---

## Epic L: Developer Experience And Integrations

### PGX-061: Publish Official SDKs

- Priority: `P2`
- Status: `proposed`
- Why:
  - Merchant adoption improves materially with supported client libraries.
- Scope:
  - at least one server SDK and one frontend SDK
  - auth, idempotency, webhook verification helpers
- Acceptance criteria:
  - SDKs are versioned and tested against the live API contract

### PGX-062: Publish Postman / Bruno / Insomnia Collections

- Priority: `P2`
- Status: `proposed`
- Why:
  - Integration testing and merchant onboarding are easier with curated request collections.
- Scope:
  - collection with env variables and example flows
- Acceptance criteria:
  - collection covers core merchant workflows and webhook simulation

### PGX-063: Build Merchant Sandbox Bootstrap CLI

- Priority: `P2`
- Status: `proposed`
- Why:
  - A CLI shortens the time from account creation to realistic test traffic.
- Scope:
  - merchant bootstrap
  - sample orders/payments/webhooks
  - local dashboard user creation
- Acceptance criteria:
  - developer can bring up a usable sandbox with one command

### PGX-064: Add Integration Recipes And Reference Apps

- Priority: `P3`
- Status: `proposed`
- Why:
  - The docs are strong, but reference apps shorten integration mistakes.
- Scope:
  - hosted checkout example
  - server-only API example
  - webhook consumer example
- Acceptance criteria:
  - at least three end-to-end sample integrations stay tested against current API

---

## Recommended Sequencing

### Wave 1

- PGX-001
- PGX-004
- PGX-009
- PGX-011
- PGX-014
- PGX-016
- PGX-017
- PGX-018
- PGX-020
- PGX-041
- PGX-042
- PGX-043

### Wave 2

- PGX-012
- PGX-019
- PGX-021
- PGX-022
- PGX-024
- PGX-025
- PGX-027
- PGX-028
- PGX-031
- PGX-033
- PGX-036
- PGX-051
- PGX-052
- PGX-053

### Wave 3

- PGX-005
- PGX-006
- PGX-007
- PGX-008
- PGX-023
- PGX-029
- PGX-030
- PGX-034
- PGX-035
- PGX-037
- PGX-038
- PGX-039
- PGX-040
- PGX-056
- PGX-057
- PGX-058
- PGX-061
- PGX-062
- PGX-063

## Notes On Current Repo Gaps Driving This Backlog

- Payment methods are broader in enums and schema than in real product behavior.
- UPI is method-labelled support today, not a full customer or merchant flow.
- Merchant onboarding is still account creation, not regulated KYB/KYC.
- Advanced platform handlers exist in code but are not fully aligned with main gateway routing and docs.
- Payouts and settlements are meaningful, but treasury, beneficiary, and approval depth are still limited.
- Risk exists, but modern fraud controls and operator workflows are still shallow.
- Reporting and merchant finance exports are materially underbuilt relative to the ledger core.

## Definition Of Done For Future Tickets

Every ticket added from this document should be considered incomplete until all of the following are true:

- runtime behavior is implemented
- API contract and docs are updated
- dashboard or operator surface is updated where applicable
- tests exist at the appropriate level:
  - unit
  - integration
  - E2E where user workflow is involved
- observability and audit implications are covered
- rollout and rollback expectations are documented
