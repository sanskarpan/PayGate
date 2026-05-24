INSERT INTO paygate_ledger.ledger_accounts (code, name, type, description)
VALUES ('MERCHANT_RESERVE_HELD', 'Merchant reserve held', 'liability', 'Funds withheld from settlement under reserve policy')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS paygate_payouts.beneficiaries (
    id                     TEXT PRIMARY KEY,
    merchant_id            TEXT NOT NULL,
    destination_type       TEXT NOT NULL CHECK (destination_type IN ('bank_account', 'vpa')),
    account_holder_name    TEXT NOT NULL,
    bank_account_last4     TEXT NOT NULL DEFAULT '',
    bank_ifsc              TEXT NOT NULL DEFAULT '',
    vpa                    TEXT NOT NULL DEFAULT '',
    fingerprint            TEXT NOT NULL,
    status                 TEXT NOT NULL CHECK (status IN ('pending_verification', 'verified', 'approved', 'rejected', 'disabled')),
    verification_fresh_until TIMESTAMPTZ,
    approved_at            TIMESTAMPTZ,
    approval_notes         TEXT NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_beneficiaries_fingerprint
    ON paygate_payouts.beneficiaries (merchant_id, fingerprint);

CREATE TABLE IF NOT EXISTS paygate_payouts.beneficiary_events (
    id              TEXT PRIMARY KEY,
    beneficiary_id  TEXT NOT NULL REFERENCES paygate_payouts.beneficiaries (id) ON DELETE CASCADE,
    merchant_id     TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    actor           TEXT NOT NULL DEFAULT '',
    actor_scope     TEXT NOT NULL DEFAULT '',
    payload_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_payouts.beneficiary_verifications (
    id                 TEXT PRIMARY KEY,
    beneficiary_id     TEXT NOT NULL REFERENCES paygate_payouts.beneficiaries (id) ON DELETE CASCADE,
    merchant_id        TEXT NOT NULL,
    provider           TEXT NOT NULL DEFAULT 'simulated',
    provider_reference TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL CHECK (status IN ('pending', 'passed', 'failed')),
    evidence_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    verified_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE paygate_payouts.payouts
    ADD COLUMN IF NOT EXISTS beneficiary_id TEXT,
    ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required' CHECK (approval_status IN ('not_required', 'pending', 'approved', 'rejected')),
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by TEXT,
    ADD COLUMN IF NOT EXISTS approval_notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS batch_id TEXT;

CREATE TABLE IF NOT EXISTS paygate_payouts.payout_approvals (
    id             TEXT PRIMARY KEY,
    payout_id      TEXT NOT NULL REFERENCES paygate_payouts.payouts (id) ON DELETE CASCADE,
    merchant_id    TEXT NOT NULL,
    actor          TEXT NOT NULL,
    actor_scope    TEXT NOT NULL DEFAULT '',
    decision       TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_payouts.payout_batches (
    id               TEXT PRIMARY KEY,
    merchant_id      TEXT NOT NULL,
    dry_run          BOOLEAN NOT NULL DEFAULT FALSE,
    status           TEXT NOT NULL CHECK (status IN ('created', 'processing', 'completed', 'partial_failed')),
    idempotency_key  TEXT NOT NULL DEFAULT '',
    summary_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_payouts.payout_batch_items (
    id               TEXT PRIMARY KEY,
    batch_id         TEXT NOT NULL REFERENCES paygate_payouts.payout_batches (id) ON DELETE CASCADE,
    merchant_id      TEXT NOT NULL,
    settlement_id    TEXT NOT NULL,
    beneficiary_id   TEXT NOT NULL,
    payout_id        TEXT,
    status           TEXT NOT NULL CHECK (status IN ('preview', 'created', 'failed')),
    error_text       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_settlements.settlement_preferences (
    merchant_id         TEXT PRIMARY KEY,
    schedule_type       TEXT NOT NULL CHECK (schedule_type IN ('daily', 'weekly', 'manual')),
    weekly_day_of_week  INTEGER CHECK (weekly_day_of_week IS NULL OR (weekly_day_of_week >= 0 AND weekly_day_of_week <= 6)),
    payout_minimum      BIGINT NOT NULL DEFAULT 0 CHECK (payout_minimum >= 0),
    approval_threshold_amount BIGINT NOT NULL DEFAULT 0 CHECK (approval_threshold_amount >= 0),
    weekend_policy      TEXT NOT NULL DEFAULT 'next_business_day' CHECK (weekend_policy IN ('next_business_day', 'same_day')),
    auto_payout         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE paygate_settlements.settlements
    ADD COLUMN IF NOT EXISTS gross_net_amount BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reserve_amount BIGINT NOT NULL DEFAULT 0;

UPDATE paygate_settlements.settlements
SET gross_net_amount = net_amount
WHERE gross_net_amount = 0;

CREATE TABLE IF NOT EXISTS paygate_settlements.settlement_reserve_releases (
    id             TEXT PRIMARY KEY,
    merchant_id    TEXT NOT NULL,
    settlement_id  TEXT NOT NULL REFERENCES paygate_settlements.settlements (id) ON DELETE CASCADE,
    amount         BIGINT NOT NULL CHECK (amount >= 0),
    release_at     TIMESTAMPTZ NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('scheduled', 'released', 'cancelled')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_settlements.settlement_statements (
    id             TEXT PRIMARY KEY,
    merchant_id    TEXT NOT NULL,
    settlement_id  TEXT NOT NULL REFERENCES paygate_settlements.settlements (id) ON DELETE CASCADE,
    format         TEXT NOT NULL CHECK (format IN ('csv')),
    file_name      TEXT NOT NULL,
    content        TEXT NOT NULL,
    totals_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_settlements.settlement_adjustments (
    id               TEXT PRIMARY KEY,
    merchant_id      TEXT NOT NULL,
    settlement_id    TEXT NOT NULL REFERENCES paygate_settlements.settlements (id) ON DELETE CASCADE,
    payment_id       TEXT NOT NULL,
    refund_id        TEXT NOT NULL,
    adjustment_type  TEXT NOT NULL CHECK (adjustment_type IN ('refund_processed', 'refund_reversed')),
    amount           BIGINT NOT NULL,
    currency         TEXT NOT NULL DEFAULT 'INR',
    reason           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
