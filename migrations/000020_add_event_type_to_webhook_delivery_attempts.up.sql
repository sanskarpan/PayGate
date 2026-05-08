ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT '';
