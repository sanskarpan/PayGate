CREATE TABLE IF NOT EXISTS paygate_sagas.saga_dead_letters (
    id             TEXT PRIMARY KEY,
    saga_id        TEXT NOT NULL REFERENCES paygate_sagas.saga_instances(id) ON DELETE CASCADE,
    step_id        TEXT REFERENCES paygate_sagas.saga_steps(id) ON DELETE SET NULL,
    merchant_id    TEXT NOT NULL,
    dead_letter_type TEXT NOT NULL CHECK (dead_letter_type IN ('command_failed', 'timeout', 'override', 'compensation_failed')),
    command_name   TEXT NOT NULL DEFAULT '',
    command_id     TEXT NOT NULL DEFAULT '',
    error_code     TEXT NOT NULL DEFAULT '',
    error_message  TEXT NOT NULL DEFAULT '',
    payload_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_saga_dead_letters_saga_created
    ON paygate_sagas.saga_dead_letters (saga_id, created_at DESC);

CREATE TABLE IF NOT EXISTS paygate_sagas.saga_operator_actions (
    id           TEXT PRIMARY KEY,
    saga_id      TEXT NOT NULL REFERENCES paygate_sagas.saga_instances(id) ON DELETE CASCADE,
    merchant_id  TEXT NOT NULL,
    action       TEXT NOT NULL,
    actor_type   TEXT NOT NULL DEFAULT 'system',
    actor_id     TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_saga_operator_actions_saga_created
    ON paygate_sagas.saga_operator_actions (saga_id, created_at DESC);
