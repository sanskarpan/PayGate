CREATE SCHEMA IF NOT EXISTS paygate_merchants;

CREATE TABLE IF NOT EXISTS paygate_merchants.merchants (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL UNIQUE,
    business_type   TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'deactivated')),
    settings        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_merchants.api_keys (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL REFERENCES paygate_merchants.merchants(id),
    secret_hash     TEXT NOT NULL,
    mode            TEXT NOT NULL CHECK (mode IN ('test', 'live')),
    scope           TEXT NOT NULL DEFAULT 'write'
                    CHECK (scope IN ('read', 'write', 'admin')),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'revoked')),
    last_used_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_merchant
    ON paygate_merchants.api_keys(merchant_id);

CREATE INDEX IF NOT EXISTS idx_api_keys_status
    ON paygate_merchants.api_keys(status)
    WHERE status = 'active';
