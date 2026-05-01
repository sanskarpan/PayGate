CREATE SCHEMA IF NOT EXISTS paygate_recon;

CREATE TABLE IF NOT EXISTS paygate_recon.recon_batches (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    batch_type      TEXT NOT NULL CHECK (batch_type IN ('ledger_balance', 'payment_ledger', 'three_way')),
    status          TEXT NOT NULL DEFAULT 'completed'
                        CHECK (status IN ('completed', 'failed')),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    checked_count   INTEGER NOT NULL DEFAULT 0,
    mismatch_count  INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recon_batches_merchant
    ON paygate_recon.recon_batches (merchant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_recon.recon_mismatches (
    id              TEXT PRIMARY KEY,
    batch_id        TEXT NOT NULL REFERENCES paygate_recon.recon_batches (id),
    merchant_id     TEXT NOT NULL,
    mismatch_type   TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    expected_value  TEXT,
    actual_value    TEXT,
    description     TEXT,
    resolved        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recon_mismatches_batch
    ON paygate_recon.recon_mismatches (batch_id);

CREATE INDEX IF NOT EXISTS idx_recon_mismatches_unresolved
    ON paygate_recon.recon_mismatches (merchant_id, resolved, created_at DESC)
    WHERE resolved = false;
