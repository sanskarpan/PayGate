CREATE TABLE IF NOT EXISTS paygate_payments.refunds (
    id                TEXT PRIMARY KEY,
    payment_id        TEXT NOT NULL REFERENCES paygate_payments.payments(id),
    order_id          TEXT NOT NULL,
    merchant_id       TEXT NOT NULL,
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          TEXT NOT NULL,
    reason            TEXT,
    status            TEXT NOT NULL DEFAULT 'created'
                          CHECK (status IN ('created', 'processing', 'processed', 'failed')),
    gateway_refund_id TEXT,
    notes             JSONB DEFAULT '{}',
    processed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refunds_payment  ON paygate_payments.refunds(payment_id);
CREATE INDEX IF NOT EXISTS idx_refunds_merchant ON paygate_payments.refunds(merchant_id, status);
