ALTER TABLE paygate_merchants.merchant_onboarding_documents
    DROP COLUMN IF EXISTS retention_updated_at,
    DROP COLUMN IF EXISTS retention_status;

ALTER TABLE paygate_webhooks.webhook_delivery_attempts
    DROP COLUMN IF EXISTS retention_updated_at,
    DROP COLUMN IF EXISTS retention_status;

ALTER TABLE paygate_reporting.export_jobs
    DROP COLUMN IF EXISTS retention_updated_at,
    DROP COLUMN IF EXISTS retention_status;

DROP TABLE IF EXISTS paygate_ops.retention_runs;
DROP TABLE IF EXISTS paygate_ops.legal_holds;
DROP TABLE IF EXISTS paygate_ops.retention_policies;
DROP SCHEMA IF EXISTS paygate_ops;
