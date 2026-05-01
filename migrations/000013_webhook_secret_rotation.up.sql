-- Add previous_secret columns to support a grace-period overlap during rotation.
-- After rotation the old secret stays valid until previous_secret_expires_at so
-- consumers have time to deploy updated verification logic.
ALTER TABLE paygate_webhooks.webhook_subscriptions
    ADD COLUMN IF NOT EXISTS previous_secret            TEXT,
    ADD COLUMN IF NOT EXISTS previous_secret_expires_at TIMESTAMPTZ;
