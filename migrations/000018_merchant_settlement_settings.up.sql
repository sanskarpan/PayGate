-- Add GIN index on merchant settings for faster JSON querying
-- Note: CONCURRENTLY omitted — golang-migrate runs migrations inside a transaction
CREATE INDEX IF NOT EXISTS idx_merchants_settings_gin
    ON paygate_merchants.merchants USING gin (settings);

-- Add comment documenting the settlement_delay_hours setting
COMMENT ON COLUMN paygate_merchants.merchants.settings IS
    'Merchant-level settings. Keys include: settlement_delay_hours (int, default 48), auto_capture (bool), fee_rate (float).';
