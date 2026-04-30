CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_users (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL REFERENCES paygate_merchants.merchants(id),
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

CREATE INDEX IF NOT EXISTS idx_merchant_users_email
    ON paygate_merchants.merchant_users(email);

CREATE SCHEMA IF NOT EXISTS paygate_idempotency;

CREATE TABLE IF NOT EXISTS paygate_idempotency.idempotency_records (
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

CREATE INDEX IF NOT EXISTS idx_idempotency_expiry
    ON paygate_idempotency.idempotency_records(expires_at);

CREATE INDEX IF NOT EXISTS idx_idempotency_in_progress
    ON paygate_idempotency.idempotency_records(locked_until)
    WHERE status = 'in_progress';

ALTER TABLE paygate_orders.orders
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency
    ON paygate_orders.orders(merchant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_attempts_idempotency
    ON paygate_payments.payment_attempts(merchant_id, order_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
