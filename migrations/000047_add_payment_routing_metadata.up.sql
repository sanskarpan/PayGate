ALTER TABLE paygate_payments.payments
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS routing_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempted_providers TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE paygate_payments.payment_attempts
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS routing_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempted_providers TEXT[] NOT NULL DEFAULT '{}';
