CREATE SCHEMA IF NOT EXISTS paygate_sagas;

CREATE TABLE IF NOT EXISTS paygate_sagas.saga_instances (
    id                 TEXT PRIMARY KEY,
    merchant_id        TEXT NOT NULL,
    saga_type          TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (status IN ('pending', 'running', 'waiting', 'compensating', 'completed', 'failed', 'aborted')),
    correlation_id     TEXT NOT NULL DEFAULT '',
    causation_id       TEXT NOT NULL DEFAULT '',
    input_payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    current_step_index INT NOT NULL DEFAULT 0,
    failure_code       TEXT,
    failure_reason     TEXT,
    leased_by          TEXT,
    last_leased_at     TIMESTAMPTZ,
    replay_count       INT NOT NULL DEFAULT 0,
    deadline_at        TIMESTAMPTZ,
    timeout_at         TIMESTAMPTZ,
    started_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS paygate_sagas.saga_steps (
    id             TEXT PRIMARY KEY,
    saga_id        TEXT NOT NULL REFERENCES paygate_sagas.saga_instances(id) ON DELETE CASCADE,
    step_index     INT NOT NULL,
    step_name      TEXT NOT NULL,
    step_kind      TEXT NOT NULL CHECK (step_kind IN ('command', 'wait', 'compensation')),
    status         TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'compensated', 'cancelled')),
    command_name   TEXT NOT NULL DEFAULT '',
    command_id     TEXT NOT NULL,
    reply_topic    TEXT NOT NULL DEFAULT '',
    input_payload  JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code     TEXT,
    error_message  TEXT,
    next_retry_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    leased_by      TEXT,
    leased_at      TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    attempt_count  INT NOT NULL DEFAULT 0,
    max_attempts   INT NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (saga_id, step_index),
    UNIQUE (command_id)
);

CREATE TABLE IF NOT EXISTS paygate_sagas.processed_commands (
    consumer_name  TEXT NOT NULL,
    command_id     TEXT NOT NULL,
    result_hash    TEXT NOT NULL,
    result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consumer_name, command_id)
);

CREATE INDEX IF NOT EXISTS idx_saga_instances_merchant_status
    ON paygate_sagas.saga_instances (merchant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_saga_steps_runnable
    ON paygate_sagas.saga_steps (status, next_retry_at, leased_at);

ALTER TABLE paygate_payouts.payouts
    ADD COLUMN IF NOT EXISTS saga_id TEXT;

CREATE INDEX IF NOT EXISTS idx_payouts_saga
    ON paygate_payouts.payouts (saga_id);
