CREATE SCHEMA IF NOT EXISTS paygate_billing;

CREATE TABLE IF NOT EXISTS paygate_billing.customers (
    id                         TEXT PRIMARY KEY,
    merchant_id                TEXT NOT NULL,
    name                       TEXT NOT NULL,
    email                      TEXT NOT NULL DEFAULT '',
    phone                      TEXT NOT NULL DEFAULT '',
    external_reference         TEXT NOT NULL DEFAULT '',
    default_payment_token_id   TEXT NOT NULL DEFAULT '',
    metadata_json              JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_customers_external_reference
    ON paygate_billing.customers (merchant_id, external_reference)
    WHERE external_reference <> '';

CREATE TABLE IF NOT EXISTS paygate_billing.subscriptions (
    id                       TEXT PRIMARY KEY,
    merchant_id              TEXT NOT NULL,
    customer_id              TEXT NOT NULL REFERENCES paygate_billing.customers (id) ON DELETE CASCADE,
    plan_name                TEXT NOT NULL,
    payment_method_token_id  TEXT NOT NULL,
    amount                   BIGINT NOT NULL CHECK (amount > 0),
    currency                 TEXT NOT NULL,
    interval_unit            TEXT NOT NULL CHECK (interval_unit IN ('day', 'week', 'month')),
    interval_count           INTEGER NOT NULL CHECK (interval_count > 0),
    status                   TEXT NOT NULL CHECK (status IN ('active', 'paused', 'canceled')),
    next_billing_at          TIMESTAMPTZ NOT NULL,
    retry_count              INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    max_retry_count          INTEGER NOT NULL DEFAULT 3 CHECK (max_retry_count >= 0),
    retry_interval_hours     INTEGER NOT NULL DEFAULT 24 CHECK (retry_interval_hours > 0),
    cancel_at_period_end     BOOLEAN NOT NULL DEFAULT FALSE,
    pause_reason             TEXT NOT NULL DEFAULT '',
    canceled_at              TIMESTAMPTZ,
    metadata_json            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_subscriptions_due
    ON paygate_billing.subscriptions (status, next_billing_at);

CREATE TABLE IF NOT EXISTS paygate_billing.invoices (
    id                TEXT PRIMARY KEY,
    merchant_id       TEXT NOT NULL,
    customer_id       TEXT NOT NULL REFERENCES paygate_billing.customers (id) ON DELETE CASCADE,
    subscription_id   TEXT NOT NULL REFERENCES paygate_billing.subscriptions (id) ON DELETE CASCADE,
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('open', 'paid', 'failed', 'void')),
    billing_reason    TEXT NOT NULL DEFAULT 'subscription_cycle',
    period_start      TIMESTAMPTZ NOT NULL,
    period_end        TIMESTAMPTZ NOT NULL,
    due_at            TIMESTAMPTZ NOT NULL,
    order_id          TEXT NOT NULL DEFAULT '',
    payment_id        TEXT NOT NULL DEFAULT '',
    failure_code      TEXT NOT NULL DEFAULT '',
    failure_message   TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_invoices_subscription
    ON paygate_billing.invoices (subscription_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_billing.invoice_attempts (
    id                  TEXT PRIMARY KEY,
    invoice_id          TEXT NOT NULL REFERENCES paygate_billing.invoices (id) ON DELETE CASCADE,
    merchant_id         TEXT NOT NULL,
    subscription_id     TEXT NOT NULL REFERENCES paygate_billing.subscriptions (id) ON DELETE CASCADE,
    attempt_number      INTEGER NOT NULL CHECK (attempt_number > 0),
    status              TEXT NOT NULL CHECK (status IN ('started', 'authorized', 'captured', 'failed')),
    order_id            TEXT NOT NULL DEFAULT '',
    payment_id          TEXT NOT NULL DEFAULT '',
    failure_code        TEXT NOT NULL DEFAULT '',
    failure_message     TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_invoice_attempts_number
    ON paygate_billing.invoice_attempts (invoice_id, attempt_number);
