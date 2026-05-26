ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN (
            'created',
            'requires_action',
            'pending_customer_action',
            'processing',
            'authorized',
            'authorization_reversed',
            'captured',
            'failed',
            'auto_refunded'
        ));

ALTER TABLE paygate_vault.card_tokens
    DROP CONSTRAINT IF EXISTS card_tokens_status_check;

ALTER TABLE paygate_vault.card_tokens
    ADD COLUMN IF NOT EXISTS reserved_payment_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reserved_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT card_tokens_status_check
        CHECK (status IN ('active', 'reserved', 'consumed', 'disabled'));

CREATE TABLE IF NOT EXISTS paygate_payments.card_challenge_sessions (
    payment_id               TEXT PRIMARY KEY REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    merchant_id              TEXT NOT NULL,
    card_token_id            TEXT NOT NULL REFERENCES paygate_vault.card_tokens(id) ON DELETE RESTRICT,
    session_id               TEXT NOT NULL UNIQUE,
    callback_token           TEXT NOT NULL,
    status                   TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed', 'abandoned', 'expired')),
    redirect_url             TEXT NOT NULL,
    challenge_reference      TEXT NOT NULL DEFAULT '',
    auto_capture_requested   BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at               TIMESTAMPTZ NOT NULL,
    completed_at             TIMESTAMPTZ NULL,
    failure_code             TEXT NOT NULL DEFAULT '',
    failure_description      TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_card_challenge_sessions_merchant_status
    ON paygate_payments.card_challenge_sessions(merchant_id, status, expires_at);
