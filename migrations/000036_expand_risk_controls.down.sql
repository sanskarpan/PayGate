DROP TABLE IF EXISTS paygate_risk.merchant_fraud_configs;

ALTER TABLE paygate_risk.risk_events
    DROP COLUMN IF EXISTS manual_decision,
    DROP COLUMN IF EXISTS review_notes,
    DROP COLUMN IF EXISTS assigned_at,
    DROP COLUMN IF EXISTS assigned_to,
    DROP COLUMN IF EXISTS review_status,
    DROP COLUMN IF EXISTS funding_type,
    DROP COLUMN IF EXISTS card_country,
    DROP COLUMN IF EXISTS issuer_country,
    DROP COLUMN IF EXISTS card_network,
    DROP COLUMN IF EXISTS card_bin,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS browser_language,
    DROP COLUMN IF EXISTS device_fingerprint_hash;
