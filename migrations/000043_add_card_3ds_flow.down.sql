DROP TABLE IF EXISTS paygate_payments.card_challenge_sessions;

ALTER TABLE paygate_vault.card_tokens
    DROP CONSTRAINT IF EXISTS card_tokens_status_check;

ALTER TABLE paygate_vault.card_tokens
    DROP COLUMN IF EXISTS reserved_at,
    DROP COLUMN IF EXISTS reserved_payment_id,
    ADD CONSTRAINT card_tokens_status_check
        CHECK (status IN ('active', 'consumed', 'disabled'));

ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN (
            'created',
            'pending_customer_action',
            'processing',
            'authorized',
            'authorization_reversed',
            'captured',
            'failed',
            'auto_refunded'
        ));
