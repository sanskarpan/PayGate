# ADR-001: Modular Monolith Boundary Strategy

**Status:** Accepted
**Date:** 2026-05-01

## Context

PayGate is a payment platform that needs to balance developer velocity, operational simplicity, and future scalability. The choice of architecture—microservices, modular monolith, or monolith—has significant long-term consequences.

Early-stage payment platforms benefit from tight coupling of concerns (orders, payments, ledger, merchants) to avoid distributed transaction complexity. However, long-term scalability requires clear domain boundaries that could be extracted later.

## Decision

Adopt a **modular monolith** where each domain (`merchant`, `order`, `payment`, `ledger`) is a self-contained Go package with:

- Its own `domain.go` (entities, errors, state machine)
- Its own `repository.go` interface + `postgres.go` implementation
- Its own `service.go` (business logic, no HTTP/gRPC coupling)
- Its own `handler.go` (HTTP adapter, no business logic)

Packages **may not** import each other directly except via:
- Explicit service constructor injection (`payment.NewService(repo, gateway, orderSvc, ledgerSvc)`)
- Shared infrastructure packages under `internal/common/`

## Consequences

**Positive:**
- No distributed transactions; all cross-domain writes happen in a single Postgres transaction
- Clear seams for future microservice extraction without large refactors
- `go test ./internal/payment/...` is fast and isolated

**Negative:**
- All code runs in one process—a bug in one domain can affect others
- Future extraction of a domain requires careful API boundary design

## Alternatives Considered

- **Microservices from day one:** Rejected due to operational overhead and distributed transaction complexity (sagas, 2PC) that is disproportionate for the current team size
- **Flat monolith:** Rejected because undisciplined cross-package imports create tight coupling that is hard to untangle later
