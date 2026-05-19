ALTER TABLE paygate_payments.payment_attempts
    DROP CONSTRAINT IF EXISTS payment_attempts_status_check;

ALTER TABLE paygate_payments.payment_attempts
    ADD CONSTRAINT payment_attempts_status_check
        CHECK (status IN ('created', 'processing', 'authorized', 'authorization_reversed', 'failed'));

ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN ('created', 'authorized', 'authorization_reversed', 'captured', 'failed', 'auto_refunded'));

ALTER TABLE paygate_payments.refunds
    DROP CONSTRAINT IF EXISTS refunds_status_check;

ALTER TABLE paygate_payments.refunds
    ADD COLUMN IF NOT EXISTS reversal_reason TEXT,
    ADD COLUMN IF NOT EXISTS reversed_at TIMESTAMPTZ;

ALTER TABLE paygate_payments.refunds
    ADD CONSTRAINT refunds_status_check
        CHECK (status IN ('created', 'processing', 'processed', 'reversed', 'failed'));

ALTER TABLE paygate_settlements.settlements
    ADD COLUMN IF NOT EXISTS rollback_marked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS rollback_reason TEXT;

ALTER TABLE paygate_payouts.payouts
    DROP CONSTRAINT IF EXISTS payouts_status_check;

ALTER TABLE paygate_payouts.payouts
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE paygate_payouts.payouts
    ADD CONSTRAINT payouts_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'returned', 'reversed', 'cancelled'));

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    DROP CONSTRAINT IF EXISTS webhook_delivery_attempts_status_check;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    ADD CONSTRAINT webhook_delivery_attempts_status_check
        CHECK (status IN ('pending', 'succeeded', 'failed', 'dead_lettered', 'cancelled'));
