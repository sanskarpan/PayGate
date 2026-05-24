CREATE SCHEMA IF NOT EXISTS paygate_ops;

CREATE TABLE IF NOT EXISTS paygate_ops.retention_policies (
    artifact_class    TEXT PRIMARY KEY,
    action            TEXT NOT NULL CHECK (action IN ('redact_content', 'redact_payload', 'redact_locator')),
    retain_days       INTEGER NOT NULL CHECK (retain_days >= 0),
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by        TEXT NOT NULL DEFAULT 'system',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_ops.legal_holds (
    id                TEXT PRIMARY KEY,
    artifact_class    TEXT NOT NULL,
    merchant_id       TEXT,
    artifact_id       TEXT,
    reason            TEXT NOT NULL,
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_legal_holds_active
    ON paygate_ops.legal_holds (artifact_class, merchant_id, artifact_id, created_at DESC)
    WHERE released_at IS NULL;

CREATE TABLE IF NOT EXISTS paygate_ops.retention_runs (
    id                TEXT PRIMARY KEY,
    artifact_class    TEXT NOT NULL,
    action            TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('started', 'completed', 'failed')),
    affected_count    INTEGER NOT NULL DEFAULT 0,
    error_message     TEXT NOT NULL DEFAULT '',
    actor_type        TEXT NOT NULL DEFAULT '',
    actor_id          TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at      TIMESTAMPTZ
);

ALTER TABLE paygate_reporting.export_jobs
    ADD COLUMN IF NOT EXISTS retention_status TEXT NOT NULL DEFAULT 'active'
        CHECK (retention_status IN ('active', 'redacted')),
    ADD COLUMN IF NOT EXISTS retention_updated_at TIMESTAMPTZ;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    ADD COLUMN IF NOT EXISTS retention_status TEXT NOT NULL DEFAULT 'active'
        CHECK (retention_status IN ('active', 'redacted')),
    ADD COLUMN IF NOT EXISTS retention_updated_at TIMESTAMPTZ;

ALTER TABLE paygate_merchants.merchant_onboarding_documents
    ADD COLUMN IF NOT EXISTS retention_status TEXT NOT NULL DEFAULT 'active'
        CHECK (retention_status IN ('active', 'redacted')),
    ADD COLUMN IF NOT EXISTS retention_updated_at TIMESTAMPTZ;

INSERT INTO paygate_ops.retention_policies (artifact_class, action, retain_days, enabled, updated_by)
VALUES
    ('report_export', 'redact_content', 30, TRUE, 'migration'),
    ('webhook_delivery_attempt', 'redact_payload', 14, TRUE, 'migration'),
    ('onboarding_document', 'redact_locator', 90, TRUE, 'migration')
ON CONFLICT (artifact_class) DO NOTHING;
