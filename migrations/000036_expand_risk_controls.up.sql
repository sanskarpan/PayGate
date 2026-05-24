ALTER TABLE paygate_risk.risk_events
    ADD COLUMN IF NOT EXISTS device_fingerprint_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS browser_language TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS card_bin TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS card_network TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS issuer_country TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS card_country TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS funding_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'not_required' CHECK (review_status IN ('not_required', 'pending', 'approved', 'blocked')),
    ADD COLUMN IF NOT EXISTS assigned_to TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS review_notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS manual_decision TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS paygate_risk.merchant_fraud_configs (
    merchant_id                 TEXT PRIMARY KEY,
    ip_velocity_threshold       INTEGER NOT NULL DEFAULT 20 CHECK (ip_velocity_threshold > 0),
    device_velocity_threshold   INTEGER NOT NULL DEFAULT 6 CHECK (device_velocity_threshold > 0),
    merchant_velocity_threshold INTEGER NOT NULL DEFAULT 200 CHECK (merchant_velocity_threshold > 0),
    amount_spike_factor         INTEGER NOT NULL DEFAULT 3 CHECK (amount_spike_factor > 0),
    review_threshold            INTEGER NOT NULL DEFAULT 40 CHECK (review_threshold >= 0),
    block_threshold             INTEGER NOT NULL DEFAULT 90 CHECK (block_threshold >= 0),
    blocked_countries           JSONB NOT NULL DEFAULT '[]'::jsonb,
    blocked_bins                JSONB NOT NULL DEFAULT '[]'::jsonb,
    review_on_country_mismatch  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
