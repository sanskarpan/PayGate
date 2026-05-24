CREATE SCHEMA IF NOT EXISTS paygate_vault;

CREATE TABLE IF NOT EXISTS paygate_vault.card_tokens (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    token_class TEXT NOT NULL CHECK (token_class IN ('single_use', 'reusable')),
    status TEXT NOT NULL CHECK (status IN ('active', 'consumed', 'disabled')),
    fingerprint_hash TEXT NOT NULL,
    last4 CHAR(4) NOT NULL,
    bin CHAR(6) NOT NULL,
    brand TEXT NOT NULL,
    exp_month INTEGER NOT NULL CHECK (exp_month BETWEEN 1 AND 12),
    exp_year INTEGER NOT NULL CHECK (exp_year BETWEEN 2000 AND 2200),
    customer_ref TEXT NOT NULL DEFAULT '',
    network_reference TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NULL,
    consumed_at TIMESTAMPTZ NULL,
    disabled_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_card_tokens_merchant_created
    ON paygate_vault.card_tokens (merchant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_card_tokens_merchant_customer
    ON paygate_vault.card_tokens (merchant_id, customer_ref)
    WHERE customer_ref <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_card_tokens_fingerprint_active
    ON paygate_vault.card_tokens (merchant_id, fingerprint_hash, token_class)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS paygate_vault.card_token_access_audit (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL REFERENCES paygate_vault.card_tokens(id) ON DELETE CASCADE,
    merchant_id TEXT NOT NULL REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    payment_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_card_token_access_audit_token_created
    ON paygate_vault.card_token_access_audit (token_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_payments.card_payment_details (
    payment_id TEXT PRIMARY KEY REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    card_token_id TEXT NOT NULL REFERENCES paygate_vault.card_tokens(id) ON DELETE RESTRICT,
    brand TEXT NOT NULL,
    last4 CHAR(4) NOT NULL,
    exp_month INTEGER NOT NULL CHECK (exp_month BETWEEN 1 AND 12),
    exp_year INTEGER NOT NULL CHECK (exp_year BETWEEN 2000 AND 2200),
    token_class TEXT NOT NULL CHECK (token_class IN ('single_use', 'reusable')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_card_payment_details_token
    ON paygate_payments.card_payment_details (card_token_id);
