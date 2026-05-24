CREATE SCHEMA IF NOT EXISTS paygate_reporting;

CREATE TABLE IF NOT EXISTS paygate_reporting.export_jobs (
    id                TEXT PRIMARY KEY,
    merchant_id       TEXT NOT NULL,
    report_type       TEXT NOT NULL,
    format            TEXT NOT NULL DEFAULT 'csv' CHECK (format IN ('csv')),
    status            TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('pending', 'completed', 'failed')),
    file_name         TEXT NOT NULL,
    content_type      TEXT NOT NULL DEFAULT 'text/csv',
    file_size_bytes   BIGINT NOT NULL DEFAULT 0,
    filters_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    content_text      TEXT NOT NULL DEFAULT '',
    download_token    TEXT NOT NULL,
    download_expires_at TIMESTAMPTZ NOT NULL,
    error_message     TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_export_jobs_merchant_created
    ON paygate_reporting.export_jobs (merchant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_reporting.tax_profiles (
    merchant_id            TEXT PRIMARY KEY,
    legal_name             TEXT NOT NULL DEFAULT '',
    gstin                  TEXT NOT NULL DEFAULT '',
    business_state_code    TEXT NOT NULL DEFAULT '',
    place_of_supply        TEXT NOT NULL DEFAULT '',
    default_tax_rate_bps   INTEGER NOT NULL DEFAULT 1800,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE paygate_recon.recon_batches
    DROP CONSTRAINT IF EXISTS recon_batches_batch_type_check;

ALTER TABLE paygate_recon.recon_batches
    ADD CONSTRAINT recon_batches_batch_type_check
    CHECK (batch_type IN ('ledger_balance', 'payment_ledger', 'three_way', 'external_source'));

CREATE TABLE IF NOT EXISTS paygate_recon.recon_source_imports (
    id              TEXT PRIMARY KEY,
    batch_id        TEXT NOT NULL REFERENCES paygate_recon.recon_batches (id),
    merchant_id     TEXT NOT NULL,
    source_type     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('pending', 'completed', 'failed')),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    entry_count     INTEGER NOT NULL DEFAULT 0,
    mismatch_count  INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recon_source_imports_merchant_created
    ON paygate_recon.recon_source_imports (merchant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_recon.recon_source_entries (
    id                TEXT PRIMARY KEY,
    source_import_id  TEXT NOT NULL REFERENCES paygate_recon.recon_source_imports (id) ON DELETE CASCADE,
    merchant_id       TEXT NOT NULL,
    entity_type       TEXT NOT NULL,
    external_id       TEXT NOT NULL,
    reference_id      TEXT NOT NULL,
    amount            BIGINT NOT NULL,
    currency          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT '',
    occurred_at       TIMESTAMPTZ NOT NULL,
    metadata_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recon_source_entries_import
    ON paygate_recon.recon_source_entries (source_import_id, created_at DESC);

ALTER TABLE paygate_recon.recon_mismatches
    ADD COLUMN IF NOT EXISTS source_import_id TEXT REFERENCES paygate_recon.recon_source_imports (id),
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'assigned', 'resolved')),
    ADD COLUMN IF NOT EXISTS assigned_to TEXT,
    ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolved_by TEXT,
    ADD COLUMN IF NOT EXISTS resolution_code TEXT,
    ADD COLUMN IF NOT EXISTS resolution_notes TEXT NOT NULL DEFAULT '';

UPDATE paygate_recon.recon_mismatches
SET status = CASE WHEN resolved THEN 'resolved' ELSE 'open' END
WHERE status IS NULL OR status = '';

CREATE TABLE IF NOT EXISTS paygate_recon.recon_mismatch_notes (
    id              TEXT PRIMARY KEY,
    mismatch_id     TEXT NOT NULL REFERENCES paygate_recon.recon_mismatches (id) ON DELETE CASCADE,
    merchant_id     TEXT NOT NULL,
    author          TEXT NOT NULL,
    note            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recon_mismatch_notes_mismatch
    ON paygate_recon.recon_mismatch_notes (mismatch_id, created_at ASC);
