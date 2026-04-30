CREATE SCHEMA IF NOT EXISTS paygate_payments;
CREATE SCHEMA IF NOT EXISTS paygate_ledger;

CREATE TABLE IF NOT EXISTS paygate_payments.payment_attempts (
    id                TEXT PRIMARY KEY,
    order_id          TEXT NOT NULL,
    merchant_id       TEXT NOT NULL,
    payment_id        TEXT,
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          TEXT NOT NULL,
    method            TEXT NOT NULL CHECK (method IN ('card', 'upi', 'netbanking', 'wallet')),
    status            TEXT NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'processing', 'authorized', 'failed')),
    gateway_reference TEXT,
    error_code        TEXT,
    error_description TEXT,
    metadata          JSONB DEFAULT '{}',
    idempotency_key   TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_payments.payments (
    id                      TEXT PRIMARY KEY,
    attempt_id              TEXT NOT NULL UNIQUE,
    order_id                TEXT NOT NULL,
    merchant_id             TEXT NOT NULL,
    amount                  BIGINT NOT NULL CHECK (amount > 0),
    currency                TEXT NOT NULL,
    method                  TEXT NOT NULL,
    status                  TEXT NOT NULL DEFAULT 'authorized' CHECK (status IN ('created','authorized', 'captured', 'failed', 'auto_refunded')),
    captured                BOOLEAN NOT NULL DEFAULT false,
    gateway_reference       TEXT,
    auth_code               TEXT,
    refund_status           TEXT DEFAULT 'none',
    amount_refunded         BIGINT NOT NULL DEFAULT 0,
    amount_refunded_pending BIGINT NOT NULL DEFAULT 0,
    settled                 BOOLEAN NOT NULL DEFAULT false,
    settlement_id           TEXT,
    fee                     BIGINT DEFAULT 0,
    tax                     BIGINT DEFAULT 0,
    notes                   JSONB DEFAULT '{}',
    error_code              TEXT,
    error_description       TEXT,
    authorized_at           TIMESTAMPTZ,
    captured_at             TIMESTAMPTZ,
    auto_capture_at         TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_order ON paygate_payments.payments(order_id);
CREATE INDEX IF NOT EXISTS idx_payments_merchant_status ON paygate_payments.payments(merchant_id, status);
CREATE INDEX IF NOT EXISTS idx_payments_autocapture ON paygate_payments.payments(auto_capture_at)
    WHERE status = 'authorized' AND auto_capture_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS paygate_ledger.ledger_accounts (
    code            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'revenue', 'expense')),
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO paygate_ledger.ledger_accounts (code, name, type, description)
VALUES
    ('CUSTOMER_RECEIVABLE', 'Customer receivable', 'asset', 'Money owed by customer'),
    ('MERCHANT_PAYABLE', 'Merchant payable', 'liability', 'Money owed to merchant'),
    ('PLATFORM_FEE_REVENUE', 'Platform fee revenue', 'revenue', 'Commission on payments'),
    ('REFUND_CLEARING', 'Refund clearing', 'liability', 'Pending refund to customer'),
    ('SETTLEMENT_CLEARING', 'Settlement clearing', 'liability', 'In-transit settlement')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS paygate_ledger.ledger_transactions (
    id              TEXT PRIMARY KEY,
    merchant_id     TEXT NOT NULL,
    source_type     TEXT NOT NULL,
    source_id       TEXT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'INR',
    total_debit     BIGINT NOT NULL CHECK (total_debit > 0),
    total_credit    BIGINT NOT NULL CHECK (total_credit > 0),
    status          TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'reversed')),
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ledger_transaction_balanced CHECK (total_debit = total_credit),
    UNIQUE (source_type, source_id)
);

CREATE TABLE IF NOT EXISTS paygate_ledger.ledger_entries (
    id              TEXT PRIMARY KEY,
    transaction_id  TEXT NOT NULL REFERENCES paygate_ledger.ledger_transactions(id),
    merchant_id     TEXT NOT NULL,
    account_code    TEXT NOT NULL REFERENCES paygate_ledger.ledger_accounts(code),
    debit_amount    BIGINT NOT NULL DEFAULT 0 CHECK (debit_amount >= 0),
    credit_amount   BIGINT NOT NULL DEFAULT 0 CHECK (credit_amount >= 0),
    currency        TEXT NOT NULL DEFAULT 'INR',
    source_type     TEXT NOT NULL,
    source_id       TEXT NOT NULL,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_single_side CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR
        (debit_amount = 0 AND credit_amount > 0)
    )
);

CREATE INDEX IF NOT EXISTS idx_ledger_transaction ON paygate_ledger.ledger_entries(transaction_id);
CREATE INDEX IF NOT EXISTS idx_ledger_merchant ON paygate_ledger.ledger_entries(merchant_id);
CREATE INDEX IF NOT EXISTS idx_ledger_account ON paygate_ledger.ledger_entries(account_code, created_at);
