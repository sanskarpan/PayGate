CREATE TABLE IF NOT EXISTS paygate_payouts.payout_simulator_scenarios (
    id                            TEXT PRIMARY KEY,
    merchant_id                   TEXT NOT NULL,
    settlement_id                 TEXT NOT NULL,
    transient_failures_remaining  INT NOT NULL DEFAULT 0,
    scenario_json                 JSONB NOT NULL DEFAULT '{"steps":[]}'::jsonb,
    notes                         TEXT NOT NULL DEFAULT '',
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, settlement_id)
);

CREATE INDEX IF NOT EXISTS idx_payout_simulator_scenarios_merchant_settlement
    ON paygate_payouts.payout_simulator_scenarios (merchant_id, settlement_id);
