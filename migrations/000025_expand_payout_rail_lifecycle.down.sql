DROP TABLE IF EXISTS paygate_payouts.payout_callback_receipts;
DROP TABLE IF EXISTS paygate_payouts.payout_events;

ALTER TABLE paygate_payouts.payouts
    DROP CONSTRAINT IF EXISTS payouts_status_check;

ALTER TABLE paygate_payouts.payouts
    DROP COLUMN IF EXISTS rail_reference,
    DROP COLUMN IF EXISTS return_reason,
    DROP COLUMN IF EXISTS failed_at,
    DROP COLUMN IF EXISTS returned_at,
    DROP COLUMN IF EXISTS reversed_at;

ALTER TABLE paygate_payouts.payouts
    ADD CONSTRAINT payouts_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed'));
