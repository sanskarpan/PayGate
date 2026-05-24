DROP TABLE IF EXISTS paygate_settlements.settlement_adjustments;
DROP TABLE IF EXISTS paygate_settlements.settlement_statements;
DROP TABLE IF EXISTS paygate_settlements.settlement_reserve_releases;
ALTER TABLE paygate_settlements.settlements
    DROP COLUMN IF EXISTS reserve_amount,
    DROP COLUMN IF EXISTS gross_net_amount;
DROP TABLE IF EXISTS paygate_settlements.settlement_preferences;
DROP TABLE IF EXISTS paygate_payouts.payout_batch_items;
DROP TABLE IF EXISTS paygate_payouts.payout_batches;
DROP TABLE IF EXISTS paygate_payouts.payout_approvals;
ALTER TABLE paygate_payouts.payouts
    DROP COLUMN IF EXISTS batch_id,
    DROP COLUMN IF EXISTS approval_notes,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approval_status,
    DROP COLUMN IF EXISTS beneficiary_id;
DROP TABLE IF EXISTS paygate_payouts.beneficiary_verifications;
DROP TABLE IF EXISTS paygate_payouts.beneficiary_events;
DROP TABLE IF EXISTS paygate_payouts.beneficiaries;
DELETE FROM paygate_ledger.ledger_accounts WHERE code = 'MERCHANT_RESERVE_HELD';
