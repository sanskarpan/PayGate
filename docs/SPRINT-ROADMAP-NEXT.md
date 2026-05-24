# Next Sprint Roadmap

This roadmap covers the highest-value remaining additions beyond the currently
implemented PayGate platform. It is intentionally biased toward product depth,
production realism, and market-ready operating controls rather than breadth for
its own sake.

The current repo is in a strong state across:

- money movement core
- async orchestration and schema control
- dashboard/operator workflows
- testing and chaos harnesses
- compliance, treasury, billing, reporting, and retention foundations

The next sprint should therefore focus on the remaining market-critical gaps:

1. full UPI productization
2. advanced card and processor capabilities
3. stronger production security and compliance controls
4. merchant and operator experience depth
5. marketplace and connected-account maturity

## Sprint Objectives

### Objective 1: Complete India-First Payment Method Depth

Deliver the missing UPI and alternate-method surfaces so PayGate supports real
merchant adoption patterns instead of only core sandbox flows.

Target tickets:

- `PGX-002` Productize UPI QR flow
- `PGX-003` Add UPI VPA validation and payee verification
- `PGX-004` Implement UPI AutoPay mandate lifecycle
- `PGX-005` Add netbanking as a real redirect method
- `PGX-006` Add wallet payment lifecycle
- `PGX-008` Build invoice and collect request surface

Definition of success:

- UPI supports intent, QR, VPA validation, and recurring mandate primitives
- method-specific UX, webhooks, reporting, and dashboard views are consistent
- alternate methods are not just enum values; they have full lifecycle behavior

## Objective 2: Finish The Card And Processor Layer

The platform now has tokenization foundations. The next sprint should turn that
into a realistic card acceptance stack.

Target tickets:

- `PGX-011` Implement 3DS / challenge flow support
- `PGX-012` Add processor routing layer
- `PGX-013` Add network token and card metadata normalization

Definition of success:

- card flows can pause in `requires_action` and resume safely
- routing decisions are explicit, persisted, and observable
- provider selection and fallback do not risk duplicate authorization

## Objective 3: Harden Production Security Beyond Foundations

Application-level encryption and signing are in place. The next sprint should
move toward stronger production-grade control boundaries.

Target tickets:

- `PGX-047` Add secret / KMS / key hierarchy strategy
  - finish the production KMS integration path beyond env-backed protection
- `PGX-048` Add field-level encryption for additional sensitive domains
  - expand coverage beyond the current merchant, webhook, payout, and tax fields
- follow-on security hardening work from the same theme:
  - rotation runbooks
  - access-boundary enforcement
  - storage lifecycle integration with object-store deletion or archival

Definition of success:

- keys are versioned, rotated, and mapped to runtime access boundaries
- sensitive business fields have consistent at-rest protection coverage
- operational guidance matches the actual production security posture

## Objective 4: Deepen Merchant And Operator Experience

The dashboard is now strong for internal operations. The next sprint should
improve high-volume usage and merchant usability.

Target tickets:

- `PGX-055` Add advanced filters, saved views, and bulk actions
- follow-on merchant UX work across:
  - onboarding review ergonomics
  - reporting/filter persistence
  - merchant-facing finance workflows

Definition of success:

- operators can work large queues without manual filter rebuilds
- review, payout, and finance workflows are efficient at higher volume
- merchant surfaces reduce API-only dependence for common tasks

## Objective 5: Expand Marketplace And Connected-Account Depth

Connected account, split, and reserve foundations exist. The next sprint should
complete the real platform model.

Target tickets:

- `PGX-029` Smart Collect / virtual accounts depth
- `PGX-030` Marketplace / split payments maturity
- `PGX-035` Reserve escalation from risk signals follow-through

Definition of success:

- smart collect can support deeper attribution and unmatched handling
- splits support more realistic fee-bearer and beneficiary behavior
- risk-triggered reserve changes are auditable and operationally usable

## Suggested Sprint Waves

### Wave A: Highest-Value Merchant Capabilities

- `PGX-002`
- `PGX-003`
- `PGX-011`
- `PGX-012`

Reason:

- these are the most direct expansion points for real payment acceptance depth

### Wave B: Recurring, Alternate Method, And Platform Growth

- `PGX-004`
- `PGX-005`
- `PGX-006`
- `PGX-029`
- `PGX-030`

Reason:

- these grow merchant product coverage after the core method gaps are closed

### Wave C: Security And Operator Maturity

- `PGX-047`
- `PGX-048`
- `PGX-055`
- `PGX-035`

Reason:

- these improve trust, governance, and sustained operability at scale

## Entry Criteria

Start only when all of the following remain green on `main`:

- `go test -count=1 ./...`
- `go vet ./...`
- `go test -count=1 -tags=integration ./tests/integration/...`
- `cd dashboard && pnpm lint && pnpm build && pnpm test:e2e`
- `./scripts/test/verify_integrations_artifacts.sh`
- `START_API=true ./scripts/test/run_load_smoke.sh`
- `START_API=true ./scripts/test/run_chaos_suite.sh`

## Exit Criteria

No sprint item counts as complete until:

- backend runtime behavior is implemented
- OpenAPI and docs are updated
- dashboard or operator surface is updated where relevant
- unit, integration, and E2E coverage are added where appropriate
- observability and audit implications are covered
- load, chaos, and rollout safety expectations are documented or tested
