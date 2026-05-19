ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    DROP CONSTRAINT IF EXISTS webhook_delivery_attempts_status_check;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS cancelled_at;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    ADD CONSTRAINT webhook_delivery_attempts_status_check
        CHECK (status IN ('pending', 'succeeded', 'failed', 'dead_lettered'));

ALTER TABLE paygate_payouts.payouts
    DROP CONSTRAINT IF EXISTS payouts_status_check;

ALTER TABLE paygate_payouts.payouts
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS cancelled_at;

ALTER TABLE paygate_payouts.payouts
    ADD CONSTRAINT payouts_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'returned', 'reversed'));

ALTER TABLE paygate_settlements.settlements
    DROP COLUMN IF EXISTS rollback_marked_at,
    DROP COLUMN IF EXISTS rollback_reason;

ALTER TABLE paygate_payments.refunds
    DROP CONSTRAINT IF EXISTS refunds_status_check;

ALTER TABLE paygate_payments.refunds
    DROP COLUMN IF EXISTS reversal_reason,
    DROP COLUMN IF EXISTS reversed_at;

ALTER TABLE paygate_payments.refunds
    ADD CONSTRAINT refunds_status_check
        CHECK (status IN ('created', 'processing', 'processed', 'failed'));

ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN ('created', 'authorized', 'captured', 'failed', 'auto_refunded'));

ALTER TABLE paygate_payments.payment_attempts
    DROP CONSTRAINT IF EXISTS payment_attempts_status_check;

ALTER TABLE paygate_payments.payment_attempts
    ADD CONSTRAINT payment_attempts_status_check
        CHECK (status IN ('created', 'processing', 'authorized', 'failed'));
