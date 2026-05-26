ALTER TABLE paygate_payments.upi_payment_details
    ADD COLUMN IF NOT EXISTS flow_type TEXT NOT NULL DEFAULT 'intent'
        CHECK (flow_type IN ('intent', 'qr')),
    ADD COLUMN IF NOT EXISTS qr_mode TEXT
        CHECK (qr_mode IS NULL OR qr_mode IN ('static', 'dynamic')),
    ADD COLUMN IF NOT EXISTS qr_payload TEXT,
    ADD COLUMN IF NOT EXISTS qr_image_url TEXT,
    ADD COLUMN IF NOT EXISTS display_name TEXT,
    ADD COLUMN IF NOT EXISTS is_reusable BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE paygate_payments.upi_payment_details
SET flow_type = 'intent'
WHERE flow_type IS NULL;
