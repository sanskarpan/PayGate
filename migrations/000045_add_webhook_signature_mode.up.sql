ALTER TABLE paygate_webhooks.webhook_subscriptions
    ADD COLUMN IF NOT EXISTS signature_mode TEXT NOT NULL DEFAULT 'compat'
    CHECK (signature_mode IN ('hmac_sha256', 'compat', 'http_message_signatures'));
