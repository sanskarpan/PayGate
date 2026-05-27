ALTER TABLE paygate_payments.card_payment_details
    DROP COLUMN IF EXISTS network_token_type,
    DROP COLUMN IF EXISTS funding_type,
    DROP COLUMN IF EXISTS card_country,
    DROP COLUMN IF EXISTS issuer_country,
    DROP COLUMN IF EXISTS issuer_name;
