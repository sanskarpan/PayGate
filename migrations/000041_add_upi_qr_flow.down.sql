ALTER TABLE paygate_payments.upi_payment_details
    DROP COLUMN IF EXISTS is_reusable,
    DROP COLUMN IF EXISTS display_name,
    DROP COLUMN IF EXISTS qr_image_url,
    DROP COLUMN IF EXISTS qr_payload,
    DROP COLUMN IF EXISTS qr_mode,
    DROP COLUMN IF EXISTS flow_type;
