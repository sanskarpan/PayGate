CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_onboarding_applications (
    id                      TEXT PRIMARY KEY,
    merchant_id             TEXT NOT NULL UNIQUE REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    legal_name              TEXT NOT NULL DEFAULT '',
    business_classification TEXT NOT NULL DEFAULT '',
    registration_number     TEXT NOT NULL DEFAULT '',
    tax_identifier          TEXT NOT NULL DEFAULT '',
    country_code            TEXT NOT NULL DEFAULT 'IN',
    state                   TEXT NOT NULL CHECK (state IN ('draft', 'submitted', 'in_review', 'needs_information', 'approved', 'rejected')),
    reviewer_notes          TEXT NOT NULL DEFAULT '',
    submitted_at            TIMESTAMPTZ NULL,
    reviewed_at             TIMESTAMPTZ NULL,
    approved_at             TIMESTAMPTZ NULL,
    rejected_at             TIMESTAMPTZ NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_onboarding_state
    ON paygate_merchants.merchant_onboarding_applications (state, updated_at DESC);

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_onboarding_events (
    id                 TEXT PRIMARY KEY,
    application_id     TEXT NOT NULL REFERENCES paygate_merchants.merchant_onboarding_applications(id) ON DELETE CASCADE,
    merchant_id        TEXT NOT NULL REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    event_type         TEXT NOT NULL,
    actor              TEXT NOT NULL DEFAULT '',
    actor_scope        TEXT NOT NULL DEFAULT '',
    state_before       TEXT NOT NULL DEFAULT '',
    state_after        TEXT NOT NULL DEFAULT '',
    payload            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_onboarding_events_application_created
    ON paygate_merchants.merchant_onboarding_events (application_id, created_at DESC);
