CREATE TABLE IF NOT EXISTS paygate_billing.upi_mandates (
    id                   TEXT PRIMARY KEY,
    merchant_id          TEXT NOT NULL,
    customer_id          TEXT NOT NULL REFERENCES paygate_billing.customers(id) ON DELETE CASCADE,
    reference            TEXT NOT NULL,
    display_name         TEXT NOT NULL,
    vpa                  TEXT NOT NULL,
    amount_limit         BIGINT NOT NULL CHECK (amount_limit > 0),
    currency             TEXT NOT NULL,
    interval_unit        TEXT NOT NULL CHECK (interval_unit IN ('day', 'week', 'month')),
    interval_count       INT NOT NULL CHECK (interval_count > 0),
    retry_window_hours   INT NOT NULL DEFAULT 24 CHECK (retry_window_hours > 0),
    status               TEXT NOT NULL CHECK (status IN ('pending_approval', 'active', 'paused', 'revoked', 'expired', 'failed')),
    approval_token       TEXT,
    approved_at          TIMESTAMPTZ,
    paused_at            TIMESTAMPTZ,
    revoked_at           TIMESTAMPTZ,
    expires_at           TIMESTAMPTZ,
    metadata_json        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, reference)
);

CREATE INDEX IF NOT EXISTS idx_upi_mandates_merchant_status
    ON paygate_billing.upi_mandates (merchant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_billing.upi_mandate_events (
    id             TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    mandate_id      TEXT NOT NULL REFERENCES paygate_billing.upi_mandates(id) ON DELETE CASCADE,
    event_type      TEXT NOT NULL,
    actor_type      TEXT NOT NULL,
    actor_id        TEXT NOT NULL,
    reason          TEXT,
    payment_id      TEXT,
    metadata_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upi_mandate_events_mandate_created
    ON paygate_billing.upi_mandate_events (mandate_id, created_at DESC);

ALTER TABLE paygate_billing.subscriptions
    ADD COLUMN IF NOT EXISTS collection_method TEXT NOT NULL DEFAULT 'card'
        CHECK (collection_method IN ('card', 'upi_mandate')),
    ADD COLUMN IF NOT EXISTS upi_mandate_id TEXT REFERENCES paygate_billing.upi_mandates(id) ON DELETE SET NULL;

ALTER TABLE paygate_payments.upi_payment_details
    DROP CONSTRAINT IF EXISTS upi_payment_details_flow_type_check;

ALTER TABLE paygate_payments.upi_payment_details
    ADD COLUMN IF NOT EXISTS mandate_id TEXT REFERENCES paygate_billing.upi_mandates(id) ON DELETE SET NULL,
    ADD CONSTRAINT upi_payment_details_flow_type_check
        CHECK (flow_type IN ('intent', 'qr', 'mandate'));
