DROP INDEX IF EXISTS paygate_payouts.idx_payouts_saga;

ALTER TABLE paygate_payouts.payouts
    DROP COLUMN IF EXISTS saga_id;

DROP INDEX IF EXISTS paygate_sagas.idx_saga_steps_runnable;
DROP INDEX IF EXISTS paygate_sagas.idx_saga_instances_merchant_status;

DROP TABLE IF EXISTS paygate_sagas.processed_commands;
DROP TABLE IF EXISTS paygate_sagas.saga_steps;
DROP TABLE IF EXISTS paygate_sagas.saga_instances;
DROP SCHEMA IF EXISTS paygate_sagas;
