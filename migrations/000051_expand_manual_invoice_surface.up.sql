ALTER TABLE paygate_billing.invoices
    DROP CONSTRAINT IF EXISTS invoices_subscription_id_fkey;

ALTER TABLE paygate_billing.invoices
    ALTER COLUMN subscription_id DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS external_reference TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payment_link_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS virtual_account_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reminder_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_reminded_at TIMESTAMPTZ;

ALTER TABLE paygate_billing.invoices
    ADD CONSTRAINT invoices_subscription_id_fkey
    FOREIGN KEY (subscription_id) REFERENCES paygate_billing.subscriptions(id) ON DELETE CASCADE;
