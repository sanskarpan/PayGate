ALTER TABLE paygate_payments.payments
    DROP COLUMN IF EXISTS method_state,
    DROP COLUMN IF EXISTS method_state_reason;
