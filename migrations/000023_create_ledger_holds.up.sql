CREATE TABLE IF NOT EXISTS paygate_ledger.ledger_holds (
    id                  TEXT PRIMARY KEY,
    merchant_id         TEXT NOT NULL,
    account_code        TEXT NOT NULL,
    source_type         TEXT NOT NULL,
    source_id           TEXT NOT NULL,
    reason              TEXT NOT NULL DEFAULT '',
    currency            TEXT NOT NULL DEFAULT 'INR',
    amount              BIGINT NOT NULL CHECK (amount > 0),
    status              TEXT NOT NULL CHECK (status IN ('active', 'released', 'committed', 'expired')),
    idempotency_key     TEXT NOT NULL DEFAULT '',
    target_account_code TEXT,
    description         TEXT NOT NULL DEFAULT '',
    expires_at          TIMESTAMPTZ,
    released_at         TIMESTAMPTZ,
    committed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_holds_merchant_status
    ON paygate_ledger.ledger_holds (merchant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ledger_holds_account_status
    ON paygate_ledger.ledger_holds (merchant_id, account_code, currency, status);
