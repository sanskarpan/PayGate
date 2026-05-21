# ADR 005: Event Schema Governance

## Status

Accepted

## Context

Phase 5 introduces durable event contracts between orchestrated flows, outbox publishing, webhook consumers, and future extracted services. Without explicit schema governance, producer changes can silently break downstream consumers or corrupt replay behavior.

## Decision

PayGate will govern event schemas through:

- registry-backed schema metadata and version history
- additive-first versioning rules
- compatibility checks for producer changes
- fixture-backed schema linting in repository tests
- consumer contract tests using real consumer code paths
- dual-publish rollouts before version activation
- envelope-level `schema_subject` and `schema_version` on published events

## Consequences

Positive:

- producer changes are visible and reviewable
- schema regressions fail in repo tests before activation
- replay logic can honor historical schema versions
- cutovers can happen progressively instead of by flag day

Tradeoffs:

- schema updates require fixture maintenance
- rollout metadata adds operational overhead
- consumers must tolerate unknown optional fields to benefit from additive evolution
