CREATE TABLE IF NOT EXISTS paygate_billing.virtual_accounts (
    id                  TEXT PRIMARY KEY,
    merchant_id         TEXT NOT NULL,
    customer_id         TEXT NOT NULL DEFAULT '',
    order_id            TEXT NOT NULL DEFAULT '',
    reference           TEXT NOT NULL,
    provider            TEXT NOT NULL DEFAULT 'simulated',
    bank_name           TEXT NOT NULL DEFAULT 'PayGate Bank',
    account_number      TEXT NOT NULL,
    ifsc                TEXT NOT NULL DEFAULT 'PGAT0001234',
    upi_vpa             TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (status IN ('active', 'inactive')) DEFAULT 'active',
    metadata_json       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_virtual_accounts_reference
    ON paygate_billing.virtual_accounts (merchant_id, reference);

CREATE UNIQUE INDEX IF NOT EXISTS idx_virtual_accounts_account_number
    ON paygate_billing.virtual_accounts (account_number);

CREATE TABLE IF NOT EXISTS paygate_billing.inbound_collections (
    id                  TEXT PRIMARY KEY,
    merchant_id         TEXT NOT NULL,
    virtual_account_id  TEXT NOT NULL REFERENCES paygate_billing.virtual_accounts (id) ON DELETE CASCADE,
    customer_id         TEXT NOT NULL DEFAULT '',
    order_id            TEXT NOT NULL DEFAULT '',
    amount              BIGINT NOT NULL CHECK (amount > 0),
    currency            TEXT NOT NULL DEFAULT 'INR',
    remitter_name       TEXT NOT NULL DEFAULT '',
    remitter_account    TEXT NOT NULL DEFAULT '',
    remitter_ifsc       TEXT NOT NULL DEFAULT '',
    remitter_vpa        TEXT NOT NULL DEFAULT '',
    utr                 TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (status IN ('matched', 'review_required')) NOT NULL,
    review_notes        TEXT NOT NULL DEFAULT '',
    matched_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_inbound_collections_utr
    ON paygate_billing.inbound_collections (merchant_id, utr)
    WHERE utr <> '';

CREATE INDEX IF NOT EXISTS idx_inbound_collections_review
    ON paygate_billing.inbound_collections (merchant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_billing.connected_accounts (
    id                    TEXT PRIMARY KEY,
    merchant_id           TEXT NOT NULL,
    linked_merchant_id    TEXT NOT NULL DEFAULT '',
    beneficiary_id        TEXT NOT NULL DEFAULT '',
    display_name          TEXT NOT NULL,
    external_reference    TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL CHECK (status IN ('active', 'inactive')) DEFAULT 'active',
    metadata_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_connected_accounts_external_reference
    ON paygate_billing.connected_accounts (merchant_id, external_reference)
    WHERE external_reference <> '';

CREATE TABLE IF NOT EXISTS paygate_payments.payment_splits (
    id                  TEXT PRIMARY KEY,
    merchant_id         TEXT NOT NULL,
    payment_id          TEXT NOT NULL REFERENCES paygate_payments.payments (id) ON DELETE CASCADE,
    destination_type    TEXT NOT NULL CHECK (destination_type IN ('merchant', 'connected_account')),
    destination_ref     TEXT NOT NULL DEFAULT '',
    beneficiary_label   TEXT NOT NULL DEFAULT '',
    amount              BIGINT NOT NULL CHECK (amount >= 0),
    currency            TEXT NOT NULL DEFAULT 'INR',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_splits_payment
    ON paygate_payments.payment_splits (payment_id, created_at);

CREATE TABLE IF NOT EXISTS paygate_merchants.reserve_escalations (
    id                        TEXT PRIMARY KEY,
    merchant_id               TEXT NOT NULL,
    risk_event_id             TEXT NOT NULL,
    trigger_score             INTEGER NOT NULL DEFAULT 0,
    triggered_rules           JSONB NOT NULL DEFAULT '[]'::jsonb,
    status                    TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')) DEFAULT 'pending',
    suggested_policy_type     TEXT NOT NULL,
    suggested_percentage_bps  INTEGER NOT NULL DEFAULT 0,
    suggested_hold_days       INTEGER NOT NULL DEFAULT 0,
    suggested_threshold_amount BIGINT NOT NULL DEFAULT 0,
    rationale                 TEXT NOT NULL DEFAULT '',
    review_notes              TEXT NOT NULL DEFAULT '',
    reviewed_by               TEXT NOT NULL DEFAULT '',
    reviewed_at               TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reserve_escalations_merchant_status
    ON paygate_merchants.reserve_escalations (merchant_id, status, created_at DESC);
