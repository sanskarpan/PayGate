ALTER TABLE paygate_merchants.merchant_onboarding_events
    ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_onboarding_parties (
    id                   TEXT PRIMARY KEY,
    application_id       TEXT NOT NULL REFERENCES paygate_merchants.merchant_onboarding_applications (id) ON DELETE CASCADE,
    merchant_id          TEXT NOT NULL,
    party_type           TEXT NOT NULL CHECK (party_type IN ('beneficial_owner', 'controller')),
    full_name            TEXT NOT NULL,
    title                TEXT NOT NULL DEFAULT '',
    email                TEXT NOT NULL DEFAULT '',
    phone                TEXT NOT NULL DEFAULT '',
    ownership_bps        INTEGER NOT NULL DEFAULT 0 CHECK (ownership_bps >= 0 AND ownership_bps <= 10000),
    verification_status  TEXT NOT NULL DEFAULT 'pending' CHECK (verification_status IN ('pending', 'verified', 'rejected')),
    evidence_notes       TEXT NOT NULL DEFAULT '',
    revision             INTEGER NOT NULL DEFAULT 1,
    superseded_at        TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_onboarding_parties_current
    ON paygate_merchants.merchant_onboarding_parties (merchant_id, party_type, created_at DESC)
    WHERE superseded_at IS NULL;

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_onboarding_documents (
    id                TEXT PRIMARY KEY,
    application_id    TEXT NOT NULL REFERENCES paygate_merchants.merchant_onboarding_applications (id) ON DELETE CASCADE,
    merchant_id       TEXT NOT NULL,
    document_type     TEXT NOT NULL,
    file_name         TEXT NOT NULL DEFAULT '',
    content_type      TEXT NOT NULL DEFAULT '',
    storage_key       TEXT NOT NULL DEFAULT '',
    request_reason    TEXT NOT NULL DEFAULT '',
    review_notes      TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'uploaded', 'approved', 'rejected', 'expired')),
    requested_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uploaded_at       TIMESTAMPTZ,
    reviewed_at       TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_onboarding_documents_current
    ON paygate_merchants.merchant_onboarding_documents (merchant_id, status, document_type, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_screening_cases (
    id                  TEXT PRIMARY KEY,
    application_id      TEXT NOT NULL REFERENCES paygate_merchants.merchant_onboarding_applications (id) ON DELETE CASCADE,
    merchant_id         TEXT NOT NULL,
    screening_type      TEXT NOT NULL DEFAULT 'kyb' CHECK (screening_type IN ('kyb', 'beneficial_owner', 'controller')),
    provider            TEXT NOT NULL DEFAULT 'simulated',
    provider_reference  TEXT NOT NULL DEFAULT '',
    subject_name        TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('pending', 'passed', 'review', 'failed')),
    result_payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    reviewed_by         TEXT NOT NULL DEFAULT '',
    screened_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_screening_cases_lookup
    ON paygate_merchants.merchant_screening_cases (merchant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_capabilities (
    id               TEXT PRIMARY KEY,
    merchant_id      TEXT NOT NULL,
    capability_code  TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('enabled', 'restricted', 'disabled')),
    reason           TEXT NOT NULL DEFAULT '',
    updated_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, capability_code)
);

CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_reserve_policies (
    id               TEXT PRIMARY KEY,
    merchant_id      TEXT NOT NULL UNIQUE,
    policy_type      TEXT NOT NULL CHECK (policy_type IN ('none', 'fixed_percentage', 'rolling_percentage')),
    percentage_bps   INTEGER NOT NULL DEFAULT 0 CHECK (percentage_bps >= 0 AND percentage_bps <= 10000),
    hold_days        INTEGER NOT NULL DEFAULT 0 CHECK (hold_days >= 0),
    threshold_amount BIGINT NOT NULL DEFAULT 0 CHECK (threshold_amount >= 0),
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
