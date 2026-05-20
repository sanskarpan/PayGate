ALTER TABLE paygate_payouts.payouts
    DROP CONSTRAINT IF EXISTS payouts_status_check;

ALTER TABLE paygate_payouts.payouts
    ADD COLUMN IF NOT EXISTS rail_reference TEXT NULL,
    ADD COLUMN IF NOT EXISTS return_reason TEXT NULL,
    ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS returned_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS reversed_at TIMESTAMPTZ NULL;

ALTER TABLE paygate_payouts.payouts
    ADD CONSTRAINT payouts_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'returned', 'reversed'));

CREATE TABLE IF NOT EXISTS paygate_payouts.payout_events (
    id                TEXT PRIMARY KEY,
    payout_id         TEXT NOT NULL REFERENCES paygate_payouts.payouts(id) ON DELETE CASCADE,
    merchant_id       TEXT NOT NULL,
    event_type        TEXT NOT NULL,
    status_before     TEXT NOT NULL,
    status_after      TEXT NOT NULL,
    callback_event_id TEXT NULL,
    payload_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payout_events_payout_created
    ON paygate_payouts.payout_events (payout_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payout_events_callback_event
    ON paygate_payouts.payout_events (callback_event_id)
    WHERE callback_event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS paygate_payouts.payout_callback_receipts (
    id                TEXT PRIMARY KEY,
    callback_event_id TEXT NOT NULL UNIQUE,
    payout_id         TEXT NOT NULL REFERENCES paygate_payouts.payouts(id) ON DELETE CASCADE,
    merchant_id       TEXT NOT NULL,
    callback_status   TEXT NOT NULL,
    signature         TEXT NOT NULL,
    payload_hash      TEXT NOT NULL,
    processed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payout_callback_receipts_payout
    ON paygate_payouts.payout_callback_receipts (payout_id, created_at DESC);
