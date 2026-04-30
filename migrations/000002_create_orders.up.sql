CREATE SCHEMA IF NOT EXISTS paygate_orders;

CREATE TABLE IF NOT EXISTS paygate_orders.orders (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    amount_paid     BIGINT NOT NULL DEFAULT 0,
    amount_due      BIGINT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'INR' CHECK (currency IN ('INR', 'USD')),
    receipt         TEXT,
    status          TEXT NOT NULL DEFAULT 'created'
                    CHECK (status IN ('created', 'attempted', 'paid', 'failed', 'expired')),
    partial_payment BOOLEAN NOT NULL DEFAULT false,
    notes           JSONB DEFAULT '{}',
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_merchant ON paygate_orders.orders(merchant_id);
CREATE INDEX IF NOT EXISTS idx_orders_merchant_status ON paygate_orders.orders(merchant_id, status);
CREATE INDEX IF NOT EXISTS idx_orders_created ON paygate_orders.orders(created_at);
CREATE INDEX IF NOT EXISTS idx_orders_receipt ON paygate_orders.orders(merchant_id, receipt);
CREATE INDEX IF NOT EXISTS idx_orders_expires ON paygate_orders.orders(expires_at) WHERE status = 'created';
