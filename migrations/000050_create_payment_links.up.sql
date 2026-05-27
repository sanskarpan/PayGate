CREATE TABLE IF NOT EXISTS paygate_billing.payment_links (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL,
    customer_id TEXT REFERENCES paygate_billing.customers(id) ON DELETE SET NULL,
    order_id TEXT NOT NULL,
    external_reference TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled', 'expired', 'paid')),
    callback_url TEXT NOT NULL DEFAULT '/checkout/callback',
    notes JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    last_visited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_payment_links_merchant_created
    ON paygate_billing.payment_links(merchant_id, created_at DESC);
