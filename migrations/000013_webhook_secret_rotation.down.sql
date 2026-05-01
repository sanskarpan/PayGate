ALTER TABLE paygate_webhooks.webhook_subscriptions
    DROP COLUMN IF EXISTS previous_secret,
    DROP COLUMN IF EXISTS previous_secret_expires_at;
