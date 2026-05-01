CREATE SCHEMA IF NOT EXISTS paygate_settlements;

CREATE TABLE IF NOT EXISTS paygate_settlements.settlements (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created'
                        CHECK (status IN ('created', 'processing', 'processed', 'failed')),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    total_amount    BIGINT NOT NULL DEFAULT 0,
    total_fees      BIGINT NOT NULL DEFAULT 0,
    total_refunds   BIGINT NOT NULL DEFAULT 0,
    net_amount      BIGINT NOT NULL DEFAULT 0,
    payment_count   INTEGER NOT NULL DEFAULT 0,
    currency        TEXT NOT NULL DEFAULT 'INR',
    processed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_settlements_merchant
    ON paygate_settlements.settlements (merchant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_settlements.settlement_items (
    id             TEXT PRIMARY KEY,
    settlement_id  TEXT NOT NULL REFERENCES paygate_settlements.settlements (id),
    payment_id     TEXT NOT NULL,
    merchant_id    TEXT NOT NULL,
    amount         BIGINT NOT NULL,
    fee            BIGINT NOT NULL DEFAULT 0,
    refunds        BIGINT NOT NULL DEFAULT 0,
    net            BIGINT NOT NULL,
    currency       TEXT NOT NULL DEFAULT 'INR',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_settlement_items_settlement
    ON paygate_settlements.settlement_items (settlement_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_settlement_items_payment
    ON paygate_settlements.settlement_items (payment_id);
