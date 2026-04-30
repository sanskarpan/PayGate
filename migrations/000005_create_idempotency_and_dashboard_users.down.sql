DROP INDEX IF EXISTS paygate_payments.idx_attempts_idempotency;
DROP INDEX IF EXISTS paygate_orders.idx_orders_idempotency;

ALTER TABLE paygate_orders.orders
    DROP COLUMN IF EXISTS idempotency_key;

DROP TABLE IF EXISTS paygate_idempotency.idempotency_records;
DROP SCHEMA IF EXISTS paygate_idempotency;

DROP INDEX IF EXISTS paygate_merchants.idx_merchant_users_email;
DROP TABLE IF EXISTS paygate_merchants.merchant_users;
