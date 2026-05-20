CREATE SCHEMA IF NOT EXISTS paygate_schema;

CREATE TABLE IF NOT EXISTS paygate_schema.event_schemas (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    topic_name TEXT NOT NULL,
    owner TEXT NOT NULL,
    review_link TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_schema.schema_versions (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL REFERENCES paygate_schema.event_schemas(subject) ON DELETE CASCADE,
    version TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'deprecated', 'retired')),
    schema_json JSONB NOT NULL,
    sample_payload JSONB NOT NULL,
    review_link TEXT NOT NULL DEFAULT '',
    compatibility_summary TEXT NOT NULL DEFAULT '',
    compatibility_details JSONB NOT NULL DEFAULT '{}'::jsonb,
    activated_at TIMESTAMPTZ,
    deprecated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (subject, version)
);

CREATE TABLE IF NOT EXISTS paygate_schema.schema_compatibility_checks (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL REFERENCES paygate_schema.event_schemas(subject) ON DELETE CASCADE,
    candidate_version TEXT NOT NULL,
    baseline_version TEXT NOT NULL,
    check_type TEXT NOT NULL CHECK (check_type IN ('backward', 'forward')),
    compatible BOOLEAN NOT NULL,
    summary TEXT NOT NULL,
    details_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_schema.schema_rollouts (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL REFERENCES paygate_schema.event_schemas(subject) ON DELETE CASCADE,
    from_version TEXT NOT NULL,
    to_version TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('planned', 'dual_publish', 'completed', 'rolled_back')),
    cutover_deadline TIMESTAMPTZ,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_schema.schema_rollout_consumers (
    id TEXT PRIMARY KEY,
    rollout_id TEXT NOT NULL REFERENCES paygate_schema.schema_rollouts(id) ON DELETE CASCADE,
    consumer_name TEXT NOT NULL,
    acknowledged_version TEXT NOT NULL,
    acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (rollout_id, consumer_name)
);

CREATE INDEX IF NOT EXISTS idx_schema_versions_subject_status
    ON paygate_schema.schema_versions(subject, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_schema_rollouts_subject_status
    ON paygate_schema.schema_rollouts(subject, status, created_at DESC);
