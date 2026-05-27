CREATE TABLE IF NOT EXISTS paygate_payments.redirect_payment_details (
    payment_id TEXT PRIMARY KEY REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    merchant_id TEXT NOT NULL,
    order_id TEXT NOT NULL,
    method TEXT NOT NULL CHECK (method IN ('netbanking', 'wallet')),
    bank_code TEXT,
    bank_name TEXT,
    wallet_code TEXT,
    wallet_name TEXT,
    redirect_url TEXT,
    gateway_reference TEXT,
    provider_status TEXT NOT NULL CHECK (provider_status IN ('pending', 'succeeded', 'failed', 'expired', 'abandoned')),
    callback_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    last_polled_at TIMESTAMPTZ,
    failure_code TEXT NOT NULL DEFAULT '',
    failure_description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redirect_payment_details_merchant_status
    ON paygate_payments.redirect_payment_details(merchant_id, provider_status, expires_at);

CREATE TABLE IF NOT EXISTS paygate_payments.redirect_callback_events (
    event_id TEXT PRIMARY KEY,
    payment_id TEXT NOT NULL REFERENCES paygate_payments.payments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
