# PayGate — Database Schema

> Complete PostgreSQL schema design. Every table, index, constraint, and migration strategy.

---

## 1. Schema organization

```
paygate_merchants   — merchant accounts, users, API keys, settings
paygate_orders      — orders
paygate_payments    — payment attempts and payments
paygate_refunds     — refunds
paygate_ledger      — double-entry ledger (append-only, most critical)
paygate_settlements — settlements and line items
paygate_webhooks    — subscriptions, events, delivery attempts
paygate_audit       — audit log (append-only)
paygate_idempotency — durable idempotency records for money-changing writes
```

Each service connects to PostgreSQL with a role that only has access to its own schema.

---

## 2. Core tables

### 2.1 Merchants schema

```sql
-- paygate_merchants schema

CREATE TABLE merchants (
    id              TEXT PRIMARY KEY,          -- merch_xxx (KSUID)
    name            TEXT NOT NULL,
    email           TEXT NOT NULL UNIQUE,
    business_type   TEXT NOT NULL,             -- individual, company, partnership
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'deactivated')),
    settings        JSONB NOT NULL DEFAULT '{}',
    -- settings contains: auto_capture (bool), capture_delay_seconds (int),
    -- settlement_cycle_days (int), platform_fee_rate (decimal),
    -- webhook_retry_enabled (bool)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE merchant_users (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL REFERENCES merchants(id),
    email           TEXT NOT NULL,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'developer', 'readonly', 'ops')),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended')),
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, email)
);

CREATE TABLE api_keys (
    id              TEXT PRIMARY KEY,          -- rzp_test_xxx or rzp_live_xxx
    merchant_id     TEXT NOT NULL REFERENCES merchants(id),
    secret_hash     TEXT NOT NULL,             -- bcrypt hash of key_secret
    mode            TEXT NOT NULL CHECK (mode IN ('test', 'live')),
    scope           TEXT NOT NULL DEFAULT 'write'
                    CHECK (scope IN ('read', 'write', 'admin')),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'revoked')),
    last_used_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_keys_merchant ON api_keys(merchant_id);
CREATE INDEX idx_api_keys_status ON api_keys(status) WHERE status = 'active';
```

### 2.2 Orders schema

```sql
-- paygate_orders schema

CREATE TABLE orders (
    id              TEXT PRIMARY KEY,          -- order_xxx (KSUID)
    merchant_id     TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),   -- in paise
    amount_paid     BIGINT NOT NULL DEFAULT 0,
    amount_due      BIGINT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'INR' CHECK (currency IN ('INR', 'USD')),
    receipt         TEXT,                      -- merchant's reference
    status          TEXT NOT NULL DEFAULT 'created'
                    CHECK (status IN ('created', 'attempted', 'paid', 'failed', 'expired')),
    partial_payment BOOLEAN NOT NULL DEFAULT false,
    notes           JSONB DEFAULT '{}',
    expires_at      TIMESTAMPTZ NOT NULL,      -- default NOW() + 30 min
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_merchant ON orders(merchant_id);
CREATE INDEX idx_orders_merchant_status ON orders(merchant_id, status);
CREATE INDEX idx_orders_created ON orders(created_at);
CREATE INDEX idx_orders_receipt ON orders(merchant_id, receipt);
CREATE INDEX idx_orders_expires ON orders(expires_at) WHERE status = 'created';
```

### 2.3 Payments schema

```sql
-- paygate_payments schema

CREATE TABLE payment_attempts (
    id              TEXT PRIMARY KEY,
    order_id        TEXT NOT NULL,
    merchant_id     TEXT NOT NULL,
    payment_id      TEXT,                     -- set when promoted to payment
    amount          BIGINT NOT NULL CHECK (amount > 0),
    currency        TEXT NOT NULL,
    method          TEXT NOT NULL CHECK (method IN ('card', 'upi', 'netbanking', 'wallet')),
    status          TEXT NOT NULL DEFAULT 'created'
                    CHECK (status IN ('created', 'processing', 'authorized', 'failed')),
    gateway_reference TEXT,                   -- external reference from gateway
    error_code      TEXT,
    error_description TEXT,
    metadata        JSONB DEFAULT '{}',
    idempotency_key TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_attempts_order ON payment_attempts(order_id);
CREATE INDEX idx_attempts_merchant ON payment_attempts(merchant_id);
CREATE UNIQUE INDEX idx_attempts_idempotency
    ON payment_attempts(merchant_id, order_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE payments (
    id              TEXT PRIMARY KEY,          -- pay_xxx (KSUID)
    attempt_id      TEXT NOT NULL UNIQUE,
    order_id        TEXT NOT NULL,
    merchant_id     TEXT NOT NULL,
    customer_id     TEXT,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    currency        TEXT NOT NULL,
    method          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'authorized'
                    CHECK (status IN ('authorized', 'captured', 'failed', 'auto_refunded')),
    captured        BOOLEAN NOT NULL DEFAULT false,
    gateway_reference TEXT,
    auth_code       TEXT,
    card_token      TEXT,                     -- tokenized card reference (never raw PAN)
    refund_status   TEXT DEFAULT 'none'
                    CHECK (refund_status IN ('none', 'partial', 'full')),
    amount_refunded BIGINT NOT NULL DEFAULT 0,
    amount_refunded_pending BIGINT NOT NULL DEFAULT 0, -- reserved by created/processing refunds
    settled         BOOLEAN NOT NULL DEFAULT false,
    settlement_id   TEXT,
    fee             BIGINT DEFAULT 0,         -- platform fee in paise
    tax             BIGINT DEFAULT 0,         -- tax on fee in paise
    notes           JSONB DEFAULT '{}',
    error_code      TEXT,
    error_description TEXT,
    authorized_at   TIMESTAMPTZ,
    captured_at     TIMESTAMPTZ,
    auto_capture_at TIMESTAMPTZ,              -- when auto-capture should fire
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payments_order ON payments(order_id);
CREATE INDEX idx_payments_merchant ON payments(merchant_id);
CREATE INDEX idx_payments_merchant_status ON payments(merchant_id, status);
CREATE INDEX idx_payments_settlement ON payments(settlement_id) WHERE settlement_id IS NOT NULL;
CREATE INDEX idx_payments_unsettled ON payments(merchant_id, captured_at)
    WHERE status = 'captured' AND settled = false;
CREATE INDEX idx_payments_autocapture ON payments(auto_capture_at)
    WHERE status = 'authorized' AND auto_capture_at IS NOT NULL;
CREATE INDEX idx_payments_created ON payments(created_at);

CREATE TABLE customers (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    email           TEXT,
    phone           TEXT,
    name            TEXT,
    notes           JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, email)
);
```

### 2.4 Refunds schema

```sql
-- paygate_refunds schema

CREATE TABLE refunds (
    id              TEXT PRIMARY KEY,          -- rfnd_xxx (KSUID)
    payment_id      TEXT NOT NULL,
    merchant_id     TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    currency        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created'
                    CHECK (status IN ('created', 'processing', 'processed', 'failed')),
    speed_requested TEXT DEFAULT 'normal'
                    CHECK (speed_requested IN ('normal', 'optimum')),
    speed_processed TEXT,
    receipt         TEXT,
    reason          TEXT,
    notes           JSONB DEFAULT '{}',
    gateway_reference TEXT,
    error_code      TEXT,
    error_description TEXT,
    idempotency_key TEXT,
    processed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refunds_payment ON refunds(payment_id);
CREATE INDEX idx_refunds_merchant ON refunds(merchant_id);
CREATE INDEX idx_refunds_status ON refunds(status) WHERE status IN ('created', 'processing');
CREATE UNIQUE INDEX idx_refunds_idempotency
    ON refunds(merchant_id, payment_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
```

### 2.5 Ledger schema

```sql
-- paygate_ledger schema
-- APPEND-ONLY: no UPDATE or DELETE permissions granted to any service role

CREATE TABLE ledger_accounts (
    code            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'revenue', 'expense')),
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed accounts
INSERT INTO ledger_accounts VALUES
    ('CUSTOMER_RECEIVABLE', 'Customer receivable', 'asset', 'Money owed by customer after payment auth'),
    ('MERCHANT_PAYABLE', 'Merchant payable', 'liability', 'Money owed to merchant from captured payments'),
    ('PLATFORM_FEE_REVENUE', 'Platform fee revenue', 'revenue', 'Commission on payments'),
    ('MERCHANT_BANK_PAYOUT', 'Merchant bank payout', 'asset', 'Funds sent to merchant bank account'),
    ('REFUND_CLEARING', 'Refund clearing', 'liability', 'Pending refund to customer'),
    ('TAX_PAYABLE', 'Tax payable', 'liability', 'GST/tax on platform fees'),
    ('SETTLEMENT_CLEARING', 'Settlement clearing', 'liability', 'In-transit settlement');

CREATE TABLE ledger_transactions (
    id              TEXT PRIMARY KEY,          -- txn_xxx (KSUID)
    merchant_id     TEXT NOT NULL,
    source_type     TEXT NOT NULL,             -- payment, refund, settlement, correction
    source_id       TEXT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'INR',
    total_debit     BIGINT NOT NULL CHECK (total_debit > 0),
    total_credit    BIGINT NOT NULL CHECK (total_credit > 0),
    status          TEXT NOT NULL DEFAULT 'posted'
                    CHECK (status IN ('posted', 'reversed')),
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_ledger_transaction_balanced CHECK (total_debit = total_credit),
    UNIQUE (source_type, source_id)
);

CREATE TABLE ledger_entries (
    id              TEXT PRIMARY KEY,          -- le_xxx (KSUID)
    transaction_id  TEXT NOT NULL REFERENCES ledger_transactions(id),
    merchant_id     TEXT NOT NULL,
    account_code    TEXT NOT NULL REFERENCES ledger_accounts(code),
    debit_amount    BIGINT NOT NULL DEFAULT 0 CHECK (debit_amount >= 0),
    credit_amount   BIGINT NOT NULL DEFAULT 0 CHECK (credit_amount >= 0),
    currency        TEXT NOT NULL DEFAULT 'INR',
    source_type     TEXT NOT NULL,             -- payment, refund, settlement
    source_id       TEXT NOT NULL,             -- pay_xxx, rfnd_xxx, sttl_xxx
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Either debit or credit, never both, never neither
    CONSTRAINT chk_single_side CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR
        (debit_amount = 0 AND credit_amount > 0)
    )
);

CREATE INDEX idx_ledger_transaction ON ledger_entries(transaction_id);
CREATE INDEX idx_ledger_txn_source ON ledger_transactions(source_type, source_id);
CREATE INDEX idx_ledger_merchant ON ledger_entries(merchant_id);
CREATE INDEX idx_ledger_source ON ledger_entries(source_type, source_id);
CREATE INDEX idx_ledger_account ON ledger_entries(account_code, created_at);
CREATE INDEX idx_ledger_created ON ledger_entries(created_at);

-- Enforce double-entry:
-- 1. Ledger module validates all rows before insert.
-- 2. ledger_transactions stores total_debit and total_credit with a CHECK constraint.
-- 3. Periodic reconciliation verifies row sums still match the transaction header:
-- SELECT lt.id, lt.total_debit, lt.total_credit,
--        SUM(le.debit_amount) AS row_debits,
--        SUM(le.credit_amount) AS row_credits
-- FROM ledger_transactions lt
-- JOIN ledger_entries le ON le.transaction_id = lt.id
-- GROUP BY lt.id
-- HAVING lt.total_debit != SUM(le.debit_amount)
--     OR lt.total_credit != SUM(le.credit_amount)
--     OR SUM(le.debit_amount) != SUM(le.credit_amount);
```

### 2.6 Settlements schema

```sql
-- paygate_settlements schema

CREATE TABLE settlements (
    id              TEXT PRIMARY KEY,          -- sttl_xxx (KSUID)
    merchant_id     TEXT NOT NULL,
    amount_gross    BIGINT NOT NULL,           -- total captured amount
    amount_fees     BIGINT NOT NULL,           -- total platform fees
    amount_tax      BIGINT NOT NULL DEFAULT 0, -- tax on fees
    amount_refunds  BIGINT NOT NULL DEFAULT 0, -- refunds deducted
    amount_net      BIGINT NOT NULL,           -- net payout amount
    currency        TEXT NOT NULL DEFAULT 'INR',
    status          TEXT NOT NULL DEFAULT 'created'
                    CHECK (status IN ('created', 'processing', 'processed', 'failed', 'on_hold')),
    payment_count   INT NOT NULL DEFAULT 0,
    cycle_start     TIMESTAMPTZ NOT NULL,
    cycle_end       TIMESTAMPTZ NOT NULL,
    utr             TEXT,                      -- bank transaction reference
    hold_reason     TEXT,
    processed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_settlements_merchant ON settlements(merchant_id);
CREATE INDEX idx_settlements_status ON settlements(status);
CREATE INDEX idx_settlements_cycle ON settlements(cycle_start, cycle_end);

CREATE TABLE settlement_items (
    id              TEXT PRIMARY KEY,
    settlement_id   TEXT NOT NULL REFERENCES settlements(id),
    payment_id      TEXT NOT NULL,
    amount_gross    BIGINT NOT NULL,
    amount_fee      BIGINT NOT NULL,
    amount_tax      BIGINT NOT NULL DEFAULT 0,
    amount_net      BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_settlement_items_settlement ON settlement_items(settlement_id);
CREATE INDEX idx_settlement_items_payment ON settlement_items(payment_id);
```

### 2.7 Webhooks schema

```sql
-- paygate_webhooks schema

CREATE TABLE webhook_subscriptions (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    url             TEXT NOT NULL,
    secret          TEXT NOT NULL,             -- used for HMAC signature
    events          TEXT[] NOT NULL,           -- array of event types to subscribe to
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'paused', 'disabled')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_subs_merchant ON webhook_subscriptions(merchant_id);

CREATE TABLE webhook_events (
    id              TEXT PRIMARY KEY,          -- evt_xxx (KSUID)
    merchant_id     TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    schema_version  TEXT NOT NULL DEFAULT '1.0.0',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_events_merchant ON webhook_events(merchant_id);
CREATE INDEX idx_webhook_events_type ON webhook_events(event_type, created_at);

CREATE TABLE webhook_delivery_attempts (
    id              TEXT PRIMARY KEY,
    webhook_event_id TEXT NOT NULL REFERENCES webhook_events(id),
    subscription_id  TEXT NOT NULL REFERENCES webhook_subscriptions(id),
    attempt_number  INT NOT NULL,
    request_url     TEXT NOT NULL,
    request_headers JSONB,
    response_status INT,
    response_body   TEXT,                     -- truncated to 1KB
    response_time_ms INT,
    error           TEXT,
    status          TEXT NOT NULL CHECK (status IN ('success', 'failed', 'timeout')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wda_event ON webhook_delivery_attempts(webhook_event_id);
CREATE INDEX idx_wda_subscription ON webhook_delivery_attempts(subscription_id);
CREATE INDEX idx_wda_status ON webhook_delivery_attempts(status, created_at)
    WHERE status = 'failed';
```

### 2.8 Outbox table

```sql
-- shared, or placed in each service's schema

CREATE TABLE outbox (
    id              TEXT PRIMARY KEY,          -- same as event ID
    aggregate_type  TEXT NOT NULL,             -- order, payment, refund, settlement
    aggregate_id    TEXT NOT NULL,             -- the entity ID
    event_type      TEXT NOT NULL,             -- payment.captured, refund.created, etc.
    payload         JSONB NOT NULL,
    merchant_id     TEXT NOT NULL,             -- partition key for Kafka
    published_at    TIMESTAMPTZ,              -- NULL until relay publishes
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_unpublished ON outbox(created_at)
    WHERE published_at IS NULL;
CREATE INDEX idx_outbox_cleanup ON outbox(published_at)
    WHERE published_at IS NOT NULL;
```

### 2.9 Audit schema

```sql
-- paygate_audit schema

CREATE TABLE audit_events (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT,
    actor_type      TEXT NOT NULL CHECK (actor_type IN ('merchant_user', 'api_key', 'system', 'ops')),
    actor_id        TEXT NOT NULL,
    action          TEXT NOT NULL,             -- payment.captured, refund.created, key.rotated, etc.
    resource_type   TEXT NOT NULL,             -- payment, refund, api_key, merchant, etc.
    resource_id     TEXT NOT NULL,
    changes         JSONB,                    -- { field: { old: x, new: y } }
    ip_address      INET,
    user_agent      TEXT,
    correlation_id  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_merchant ON audit_events(merchant_id, created_at);
CREATE INDEX idx_audit_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_audit_actor ON audit_events(actor_type, actor_id, created_at);
CREATE INDEX idx_audit_action ON audit_events(action, created_at);
```

### 2.10 Idempotency records

```sql
-- shared schema or each write-owning schema
-- Redis is a cache for idempotency. This table is the durable source of truth
-- for money-changing POST endpoints.

CREATE TABLE idempotency_records (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    endpoint_hash   TEXT NOT NULL,
    client_key      TEXT NOT NULL,
    request_hash    TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('in_progress', 'completed', 'failed')),
    resource_type   TEXT,
    resource_id     TEXT,
    response_code   INT,
    response_body   JSONB,
    locked_until    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, endpoint_hash, client_key)
);

CREATE INDEX idx_idempotency_expiry ON idempotency_records(expires_at);
CREATE INDEX idx_idempotency_in_progress ON idempotency_records(locked_until)
    WHERE status = 'in_progress';
```

### 2.11 Reconciliation schema

```sql
CREATE TABLE reconciliation_batches (
    id              TEXT PRIMARY KEY,
    batch_type      TEXT NOT NULL CHECK (batch_type IN ('continuous', 'hourly', 'nightly', 'weekly')),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    total_payments  INT NOT NULL DEFAULT 0,
    matched         INT NOT NULL DEFAULT 0,
    mismatches      INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    summary         JSONB,                    -- { by_type: { MISSING_LEDGER: 2, ... } }
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE TABLE reconciliation_mismatches (
    id              TEXT PRIMARY KEY,
    batch_id        TEXT NOT NULL REFERENCES reconciliation_batches(id),
    mismatch_type   TEXT NOT NULL,
    severity        TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    payment_id      TEXT,
    settlement_id   TEXT,
    ledger_entry_id TEXT,
    expected_value  TEXT,
    actual_value    TEXT,
    description     TEXT,
    resolved        BOOLEAN NOT NULL DEFAULT false,
    resolved_by     TEXT,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_recon_mismatches_batch ON reconciliation_mismatches(batch_id);
CREATE INDEX idx_recon_mismatches_unresolved ON reconciliation_mismatches(mismatch_type)
    WHERE resolved = false;
```

### 2.12 Risk and feature flags

```sql
CREATE TABLE risk_events (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    payment_id      TEXT,
    event_type      TEXT NOT NULL,             -- velocity_breach, amount_spike, ip_mismatch, etc.
    severity        TEXT NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    details         JSONB NOT NULL,
    action_taken    TEXT,                      -- blocked, flagged, allowed
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE feature_flags (
    key             TEXT PRIMARY KEY,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    merchant_ids    TEXT[],                   -- NULL = all merchants
    rollout_pct     INT NOT NULL DEFAULT 0 CHECK (rollout_pct BETWEEN 0 AND 100),
    description     TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 2.13 Advanced distributed schema (optional track)

```sql
CREATE TABLE saga_instances (
    id              TEXT PRIMARY KEY,
    saga_type       TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'compensating')),
    context         JSONB NOT NULL DEFAULT '{}',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE TABLE saga_steps (
    id              TEXT PRIMARY KEY,
    saga_id         TEXT NOT NULL REFERENCES saga_instances(id),
    step_name       TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'compensated')),
    command_id      TEXT NOT NULL,
    error_code      TEXT,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (command_id)
);

CREATE TABLE processed_commands (
    command_id      TEXT PRIMARY KEY,
    consumer_name   TEXT NOT NULL,
    result_hash     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE event_schemas (
    id              TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    version         TEXT NOT NULL,
    compatibility   TEXT NOT NULL CHECK (compatibility IN ('backward', 'forward', 'full', 'breaking')),
    schema_json     JSONB NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('draft', 'active', 'deprecated')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_type, version)
);

CREATE TABLE ledger_holds (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    source_type     TEXT NOT NULL,            -- risk_hold, dispute_hold, payout_hold
    source_id       TEXT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'INR',
    amount          BIGINT NOT NULL CHECK (amount > 0),
    status          TEXT NOT NULL CHECK (status IN ('active', 'released', 'committed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saga_status ON saga_instances(status);
CREATE INDEX idx_saga_step_saga ON saga_steps(saga_id);
CREATE INDEX idx_event_schemas_type ON event_schemas(event_type, status);
CREATE INDEX idx_ledger_holds_merchant ON ledger_holds(merchant_id, status);
```

---

## 3. Migration strategy

### 3.1 Tool

Use `golang-migrate/migrate` with sequential integer versioning:

```
migrations/
├── 000001_create_merchants.up.sql
├── 000001_create_merchants.down.sql
├── 000002_create_orders.up.sql
├── 000002_create_orders.down.sql
├── ...
```

### 3.2 Rules

1. **Forward-only in production** — down migrations exist for development only.
2. **Backward compatible** — never DROP, RENAME, or change column types in the same release as a code change. Use a two-phase approach: phase 1 adds the new column, phase 2 removes the old column (next release).
3. **Migrations run as a Kubernetes Job** before the service deployment starts.
4. **Every migration is tested** — the CI pipeline runs all migrations against a fresh database and verifies schema matches expectations.
5. **Lock timeout** — all DDL statements include `SET lock_timeout = '5s'` to avoid blocking production traffic.

### 3.3 Index creation

All indexes on large tables use `CREATE INDEX CONCURRENTLY` to avoid table locks:

```sql
-- In migration file:
SET lock_timeout = '5s';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payments_created
    ON payments(created_at);
```
