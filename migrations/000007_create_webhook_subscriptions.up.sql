CREATE SCHEMA IF NOT EXISTS paygate_webhooks;

CREATE TABLE IF NOT EXISTS paygate_webhooks.webhook_subscriptions (
    id          TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL,
    url         TEXT NOT NULL,
    events      TEXT[] NOT NULL DEFAULT '{}',
    secret      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'disabled', 'deleted')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_merchant_status
    ON paygate_webhooks.webhook_subscriptions (merchant_id, status);

CREATE TABLE IF NOT EXISTS paygate_webhooks.webhook_delivery_attempts (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL,
    subscription_id TEXT NOT NULL REFERENCES paygate_webhooks.webhook_subscriptions (id),
    merchant_id     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'succeeded', 'failed', 'dead_lettered')),
    request_url     TEXT NOT NULL,
    request_body    BYTEA,
    response_code   INTEGER,
    response_body   TEXT,
    error_message   TEXT,
    attempt_number  INTEGER NOT NULL DEFAULT 1,
    next_retry_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription
    ON paygate_webhooks.webhook_delivery_attempts (subscription_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retry
    ON paygate_webhooks.webhook_delivery_attempts (next_retry_at)
    WHERE status = 'failed' AND next_retry_at IS NOT NULL;
