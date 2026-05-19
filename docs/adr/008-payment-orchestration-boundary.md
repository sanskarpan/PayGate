# ADR 008: Payment Orchestration Boundary

## Decision

Payment authorization and capture remain the synchronous consistency boundary for money movement. If extracted, the command contract must preserve durable local write -> external side effect -> durable finalization semantics.

## Why

- payment state, ledger entries, and outbox publication must not split into dual-write failure modes
- retries and replay must not create duplicate authorizations or captures

## Consequence

Payment orchestration must use idempotent command handlers, explicit replay controls, and compensation semantics for irreversible gateway behavior.
