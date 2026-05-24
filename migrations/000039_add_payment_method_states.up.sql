ALTER TABLE paygate_payments.payments
    ADD COLUMN IF NOT EXISTS method_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS method_state_reason TEXT NOT NULL DEFAULT '';
