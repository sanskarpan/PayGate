DROP TABLE IF EXISTS paygate_payments.upi_callback_events;
DROP TABLE IF EXISTS paygate_payments.upi_payment_details;

ALTER TABLE paygate_payments.payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE paygate_payments.payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN (
            'created',
            'authorized',
            'authorization_reversed',
            'captured',
            'failed',
            'auto_refunded'
        ));
