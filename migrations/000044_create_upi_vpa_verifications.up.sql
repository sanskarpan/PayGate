CREATE TABLE IF NOT EXISTS paygate_billing.vpa_verifications (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    vpa TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK (purpose IN ('mandate', 'payout_destination')),
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('verified', 'rejected')),
    provider TEXT NOT NULL,
    provider_reference TEXT NOT NULL DEFAULT '',
    evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    verified_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, vpa, purpose, version)
);

CREATE INDEX IF NOT EXISTS idx_vpa_verifications_lookup
    ON paygate_billing.vpa_verifications (merchant_id, vpa, purpose, version DESC);
