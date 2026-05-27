ALTER TABLE paygate_payments.payment_attempts
    DROP COLUMN IF EXISTS attempted_providers,
    DROP COLUMN IF EXISTS routing_reason,
    DROP COLUMN IF EXISTS provider;

ALTER TABLE paygate_payments.payments
    DROP COLUMN IF EXISTS attempted_providers,
    DROP COLUMN IF EXISTS routing_reason,
    DROP COLUMN IF EXISTS provider;
