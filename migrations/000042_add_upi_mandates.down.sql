ALTER TABLE paygate_payments.upi_payment_details
    DROP CONSTRAINT IF EXISTS upi_payment_details_flow_type_check;

ALTER TABLE paygate_payments.upi_payment_details
    DROP COLUMN IF EXISTS mandate_id;

ALTER TABLE paygate_payments.upi_payment_details
    ADD CONSTRAINT upi_payment_details_flow_type_check
        CHECK (flow_type IN ('intent', 'qr'));

ALTER TABLE paygate_billing.subscriptions
    DROP COLUMN IF EXISTS upi_mandate_id,
    DROP COLUMN IF EXISTS collection_method;

DROP TABLE IF EXISTS paygate_billing.upi_mandate_events;
DROP TABLE IF EXISTS paygate_billing.upi_mandates;
