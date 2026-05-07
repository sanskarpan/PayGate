CREATE SCHEMA IF NOT EXISTS paygate_disputes;

CREATE TABLE IF NOT EXISTS paygate_disputes.disputes (
    id                    TEXT PRIMARY KEY,
    merchant_id           TEXT NOT NULL REFERENCES paygate_merchants.merchants (id),
    payment_id            TEXT NOT NULL,
    settlement_id         TEXT NULL,
    status                TEXT NOT NULL DEFAULT 'open'
                              CHECK (status IN ('open', 'under_review', 'won', 'lost', 'accepted')),
    reason                TEXT NOT NULL,
    amount                BIGINT NOT NULL,
    currency              TEXT NOT NULL DEFAULT 'INR',
    evidence              JSONB NULL,
    evidence_submitted_at TIMESTAMPTZ NULL,
    due_by                TIMESTAMPTZ NULL,
    resolved_at           TIMESTAMPTZ NULL,
    notes                 TEXT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_disputes_merchant
    ON paygate_disputes.disputes (merchant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_disputes_payment
    ON paygate_disputes.disputes (payment_id);
