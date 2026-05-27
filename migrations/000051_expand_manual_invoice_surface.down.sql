ALTER TABLE paygate_billing.invoices
    DROP CONSTRAINT IF EXISTS invoices_subscription_id_fkey;

ALTER TABLE paygate_billing.invoices
    DROP COLUMN IF EXISTS last_reminded_at,
    DROP COLUMN IF EXISTS reminder_count,
    DROP COLUMN IF EXISTS virtual_account_id,
    DROP COLUMN IF EXISTS payment_link_id,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS external_reference;

ALTER TABLE paygate_billing.invoices
    ALTER COLUMN subscription_id SET NOT NULL;

ALTER TABLE paygate_billing.invoices
    ADD CONSTRAINT invoices_subscription_id_fkey
    FOREIGN KEY (subscription_id) REFERENCES paygate_billing.subscriptions(id) ON DELETE CASCADE;
