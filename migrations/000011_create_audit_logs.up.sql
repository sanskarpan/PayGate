CREATE SCHEMA IF NOT EXISTS paygate_audit;

CREATE TABLE IF NOT EXISTS paygate_audit.audit_logs (
    id             TEXT PRIMARY KEY,
    merchant_id    TEXT        NOT NULL,
    actor_id       TEXT        NOT NULL,
    actor_email    TEXT        NOT NULL DEFAULT '',
    actor_type     TEXT        NOT NULL CHECK (actor_type IN ('dashboard_user','api_key','system')),
    action         TEXT        NOT NULL,
    resource_type  TEXT        NOT NULL,
    resource_id    TEXT        NOT NULL,
    changes        JSONB       NOT NULL DEFAULT '{}',
    ip_address     TEXT        NOT NULL DEFAULT '',
    correlation_id TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_merchant_id
    ON paygate_audit.audit_logs(merchant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
    ON paygate_audit.audit_logs(merchant_id, resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor
    ON paygate_audit.audit_logs(merchant_id, actor_id);
