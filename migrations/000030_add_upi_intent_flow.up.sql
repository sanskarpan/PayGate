ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN (
            'created',
            'pending_customer_action',
            'processing',
            'authorized',
            'authorization_reversed',
            'captured',
            'failed',
            'auto_refunded'
        ));

CREATE TABLE IF NOT EXISTS paygate_payments.upi_payment_details (
    payment_id           TEXT PRIMARY KEY REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    merchant_id          TEXT NOT NULL,
    order_id             TEXT NOT NULL,
    vpa                  TEXT NOT NULL,
    deep_link            TEXT,
    intent_uri           TEXT,
    gateway_reference    TEXT,
    provider_status      TEXT NOT NULL CHECK (provider_status IN ('pending', 'succeeded', 'failed', 'expired', 'abandoned')),
    callback_token       TEXT NOT NULL,
    expires_at           TIMESTAMPTZ NOT NULL,
    last_polled_at       TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    failure_code         TEXT,
    failure_description  TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upi_payment_details_merchant_status
    ON paygate_payments.upi_payment_details(merchant_id, provider_status, expires_at);

CREATE TABLE IF NOT EXISTS paygate_payments.upi_callback_events (
    event_id      TEXT PRIMARY KEY,
    payment_id    TEXT NOT NULL REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
