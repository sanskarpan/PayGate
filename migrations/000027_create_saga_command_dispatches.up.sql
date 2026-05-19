CREATE TABLE IF NOT EXISTS paygate_sagas.saga_command_dispatches (
    id                    TEXT PRIMARY KEY,
    saga_id               TEXT NOT NULL REFERENCES paygate_sagas.saga_instances(id) ON DELETE CASCADE,
    step_id               TEXT NOT NULL REFERENCES paygate_sagas.saga_steps(id) ON DELETE CASCADE,
    merchant_id           TEXT NOT NULL,
    command_name          TEXT NOT NULL,
    command_id            TEXT NOT NULL,
    dispatch_attempt      INT NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('dispatched', 'acked', 'nacked')),
    leased_by             TEXT NOT NULL DEFAULT '',
    leased_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acked_at              TIMESTAMPTZ,
    nacked_at             TIMESTAMPTZ,
    retry_backoff_seconds INT NOT NULL DEFAULT 0,
    error_code            TEXT NOT NULL DEFAULT '',
    error_message         TEXT NOT NULL DEFAULT '',
    error_classification  TEXT NOT NULL DEFAULT '',
    input_payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (step_id, dispatch_attempt)
);

CREATE INDEX IF NOT EXISTS idx_saga_command_dispatches_saga_created
    ON paygate_sagas.saga_command_dispatches (saga_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_saga_command_dispatches_step_status
    ON paygate_sagas.saga_command_dispatches (step_id, status, created_at DESC);
