#!/usr/bin/env bash
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5432/paygate?sslmode=disable}"
TMP_DB="${TMP_DB:-paygate_restore_verify}"
DUMP_FILE="${DUMP_FILE:-/tmp/paygate-backup.dump}"

cleanup() {
  dropdb --if-exists "${TMP_DB}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Creating logical backup from ${DATABASE_URL}"
pg_dump --format=custom --file "${DUMP_FILE}" "${DATABASE_URL}"

echo "Recreating temporary restore database ${TMP_DB}"
dropdb --if-exists "${TMP_DB}" >/dev/null 2>&1 || true
createdb "${TMP_DB}"
pg_restore --no-owner --no-privileges --dbname "${TMP_DB}" "${DUMP_FILE}"

echo "Running restore verification queries"
psql "${TMP_DB}" -c "SELECT COUNT(*) AS merchants FROM paygate_merchants.merchants;"
psql "${TMP_DB}" -c "SELECT COUNT(*) AS outbox_rows FROM public.outbox;"
psql "${TMP_DB}" -c "SELECT COUNT(*) AS payouts FROM paygate_payouts.payouts;"
psql "${TMP_DB}" -c "SELECT COUNT(*) AS saga_instances FROM paygate_sagas.saga_instances;"

echo "Backup verification completed successfully"
