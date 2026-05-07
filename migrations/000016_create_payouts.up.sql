CREATE SCHEMA IF NOT EXISTS paygate_payouts;

CREATE TABLE IF NOT EXISTS paygate_payouts.payouts (
    id               TEXT PRIMARY KEY,
    merchant_id      TEXT NOT NULL,
    settlement_id    TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    amount           BIGINT NOT NULL,
    currency         TEXT NOT NULL DEFAULT 'INR',
    bank_reference   TEXT NULL,
    failure_reason   TEXT NULL,
    initiated_at     TIMESTAMPTZ NULL,
    completed_at     TIMESTAMPTZ NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payouts_merchant
    ON paygate_payouts.payouts (merchant_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payouts_settlement
    ON paygate_payouts.payouts (settlement_id);
