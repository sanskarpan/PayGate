CREATE SCHEMA IF NOT EXISTS paygate_risk;

-- Risk events: one row per payment risk evaluation.
CREATE TABLE IF NOT EXISTS paygate_risk.risk_events (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT        NOT NULL,
    payment_id      TEXT        NOT NULL,
    score           INT         NOT NULL DEFAULT 0,
    action          TEXT        NOT NULL CHECK (action IN ('allow', 'hold', 'block')),
    triggered_rules JSONB       NOT NULL DEFAULT '[]',
    resolved        BOOLEAN     NOT NULL DEFAULT FALSE,
    resolved_by     TEXT,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_risk_events_merchant_payment
    ON paygate_risk.risk_events(merchant_id, payment_id);
CREATE INDEX IF NOT EXISTS idx_risk_events_unresolved
    ON paygate_risk.risk_events(merchant_id, resolved, created_at DESC)
    WHERE resolved = FALSE;

-- Velocity counters: rolling window counters keyed by dimension.
-- dimension: merchant_id, ip_address, card_token
-- window_type: 1h, 24h
CREATE TABLE IF NOT EXISTS paygate_risk.velocity_counters (
    id          TEXT PRIMARY KEY,
    dimension   TEXT        NOT NULL,
    dim_value   TEXT        NOT NULL,
    window_type TEXT        NOT NULL CHECK (window_type IN ('1h','24h')),
    count       INT         NOT NULL DEFAULT 1,
    amount      BIGINT      NOT NULL DEFAULT 0,
    window_end  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (dimension, dim_value, window_type, window_end)
);

CREATE INDEX IF NOT EXISTS idx_velocity_dimension
    ON paygate_risk.velocity_counters(dimension, dim_value, window_end);
