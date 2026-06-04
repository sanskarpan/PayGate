ALTER TABLE paygate_payments.payment_attempts
    DROP CONSTRAINT IF EXISTS payment_attempts_status_check;

ALTER TABLE paygate_payments.payment_attempts
    ADD CONSTRAINT payment_attempts_status_check
        CHECK (status IN ('created', 'processing', 'authorized', 'authorization_reversed', 'failed'));
