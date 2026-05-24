DROP TABLE IF EXISTS paygate_recon.recon_mismatch_notes;

ALTER TABLE paygate_recon.recon_mismatches
    DROP COLUMN IF EXISTS source_import_id,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS assigned_to,
    DROP COLUMN IF EXISTS assigned_at,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolved_by,
    DROP COLUMN IF EXISTS resolution_code,
    DROP COLUMN IF EXISTS resolution_notes;

DROP TABLE IF EXISTS paygate_recon.recon_source_entries;
DROP TABLE IF EXISTS paygate_recon.recon_source_imports;

ALTER TABLE paygate_recon.recon_batches
    DROP CONSTRAINT IF EXISTS recon_batches_batch_type_check;

ALTER TABLE paygate_recon.recon_batches
    ADD CONSTRAINT recon_batches_batch_type_check
    CHECK (batch_type IN ('ledger_balance', 'payment_ledger', 'three_way'));

DROP TABLE IF EXISTS paygate_reporting.tax_profiles;
DROP TABLE IF EXISTS paygate_reporting.export_jobs;
DROP SCHEMA IF EXISTS paygate_reporting;
