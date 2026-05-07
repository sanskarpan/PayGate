CREATE SCHEMA IF NOT EXISTS paygate_gateway;

CREATE TABLE IF NOT EXISTS paygate_gateway.gateway_scenarios (
    id           TEXT PRIMARY KEY,
    merchant_id  TEXT NOT NULL DEFAULT '',
    mode         TEXT NOT NULL DEFAULT 'success'
                     CHECK (mode IN ('success', 'slow', 'flaky', 'timeout', 'decline', 'late_callback')),
    failure_rate NUMERIC(3,2) NOT NULL DEFAULT 0.30,
    delay_ms     INTEGER NOT NULL DEFAULT 3000,
    decline_code TEXT NOT NULL DEFAULT 'CARD_DECLINED',
    active       BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gateway_scenarios_merchant
    ON paygate_gateway.gateway_scenarios (merchant_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_scenarios_active_merchant
    ON paygate_gateway.gateway_scenarios (merchant_id)
    WHERE active = true;
