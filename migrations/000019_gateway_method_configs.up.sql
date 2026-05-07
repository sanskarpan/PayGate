CREATE TABLE IF NOT EXISTS paygate_gateway.method_configs (
    id          TEXT        PRIMARY KEY,
    merchant_id TEXT        NOT NULL DEFAULT '',
    method      TEXT        NOT NULL CHECK (method IN ('card','upi','netbanking','wallet')),
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    success_rate FLOAT      NOT NULL DEFAULT 1.0 CHECK (success_rate BETWEEN 0 AND 1),
    decline_code TEXT       NOT NULL DEFAULT '',
    delay_ms    INTEGER     NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_method_configs_merchant_method
    ON paygate_gateway.method_configs(merchant_id, method);
COMMENT ON TABLE paygate_gateway.method_configs IS
    'Per-merchant, per-method gateway simulation overrides.';
