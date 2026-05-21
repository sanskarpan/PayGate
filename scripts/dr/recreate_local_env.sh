#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DB_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable}"
RESET_DATABASE="${RESET_DATABASE:-true}"

base_url="${DB_URL%%\?*}"
query_suffix=""
if [[ "${DB_URL}" == *\?* ]]; then
  query_suffix="?${DB_URL#*\?}"
fi
db_name="${base_url##*/}"
admin_url="${base_url%/*}/postgres${query_suffix}"

cd "${ROOT_DIR}"

echo "Starting local infrastructure"
docker compose up -d postgres redis kafka minio prometheus alertmanager grafana

echo "Waiting for Postgres"
until pg_isready -d "${DB_URL}" >/dev/null 2>&1; do
  sleep 2
done

if [[ "${RESET_DATABASE}" == "true" ]]; then
  echo "Resetting local database ${db_name}"
  psql "${admin_url}" -v ON_ERROR_STOP=1 <<SQL >/dev/null
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = '${db_name}' AND pid <> pg_backend_pid();

DROP DATABASE IF EXISTS "${db_name}";
CREATE DATABASE "${db_name}";
SQL
fi

echo "Applying migrations"
if command -v migrate >/dev/null 2>&1; then
  migrate -path "${ROOT_DIR}/migrations" -database "${DB_URL}" up >/dev/null
else
  go run github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1 \
    -path "${ROOT_DIR}/migrations" \
    -database "${DB_URL}" \
    up >/dev/null
fi

echo "Local environment recreated successfully"
