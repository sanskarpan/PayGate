# Schema Governance

## Versioning policy

- Producer schema changes are additive-only by default.
- New required fields are treated as breaking changes.
- Removing an existing field is treated as breaking even when the field was optional.
- Type changes are treated as breaking.
- Optional field additions are allowed when current consumers can ignore unknown fields.
- Every schema version must include:
  - `event_id`
  - `event_type`
  - `occurred_at`
  - `correlation_id`
  - `causation_id`
  - `schema_version`
  - `merchant_id`
  - `payload`

## Review and rollout

- New schema versions start in `draft`.
- A version is activated only after:
  - fixture lint passes
  - compatibility checks pass
  - consumer contract tests pass
- Producer cutover uses dual-publish when an active rollout exists.
- Consumers acknowledge readiness against a rollout before activation.

## Emergency rollback and consumer freeze

- If a newly activated version causes deserialization or business errors:
  - stop activating new versions
  - keep producers on dual-publish if a prior version still exists
  - activate the last known good version
  - freeze rollout acknowledgements until contract failures are resolved
- Deprecated versions should remain observable through publish and consumer-version metrics until cutover is complete.

## Replay safety

- Historical events must carry `schema_subject` and `schema_version` in the envelope.
- Replayers and consumers must use the historical schema version embedded in the event, not infer the current active version.
- Replayed webhook deliveries should preserve the original schema metadata.

## Fixtures and tests

- Canonical fixtures live under `schemas/events/<event_type>/`.
- Each version should include:
  - `<version>.schema.json`
  - `<version>.sample.json`
- `go test ./internal/eventschema` is the repository-level gate for fixture lint and consumer contract validation.
