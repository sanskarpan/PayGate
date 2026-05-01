-- Team invitations
CREATE TABLE IF NOT EXISTS paygate_merchants.merchant_invitations (
    id          TEXT PRIMARY KEY,
    merchant_id TEXT        NOT NULL REFERENCES paygate_merchants.merchants(id) ON DELETE CASCADE,
    email       TEXT        NOT NULL,
    role        TEXT        NOT NULL CHECK (role IN ('admin','developer','readonly','ops')),
    token_hash  TEXT        NOT NULL UNIQUE,
    status      TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','expired','revoked')),
    invited_by  TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_merchant_invitations_merchant_id
    ON paygate_merchants.merchant_invitations(merchant_id);
CREATE INDEX IF NOT EXISTS idx_merchant_invitations_email
    ON paygate_merchants.merchant_invitations(merchant_id, email);

-- IP allowlist per API key (empty array = no restriction)
ALTER TABLE paygate_merchants.api_keys
    ADD COLUMN IF NOT EXISTS allowed_ips TEXT[] NOT NULL DEFAULT '{}';
