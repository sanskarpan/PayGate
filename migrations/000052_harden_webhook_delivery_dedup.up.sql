WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY event_id, subscription_id
               ORDER BY created_at DESC, id DESC
           ) AS rn
    FROM paygate_webhooks.webhook_delivery_attempts
)
DELETE FROM paygate_webhooks.webhook_delivery_attempts attempt
USING ranked
WHERE attempt.id = ranked.id
  AND ranked.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_deliveries_event_subscription
    ON paygate_webhooks.webhook_delivery_attempts (event_id, subscription_id);
