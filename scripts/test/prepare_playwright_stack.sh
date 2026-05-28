#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5433/paygate_test?sslmode=disable}"

cd "${ROOT_DIR}"

docker compose -f docker-compose.test.yml up -d postgres-test redis-test >/dev/null

until docker exec paygate-postgres-test pg_isready -U paygate -d postgres >/dev/null 2>&1; do
  sleep 1
done

until docker exec paygate-redis-test redis-cli ping >/dev/null 2>&1; do
  sleep 1
done

DATABASE_NAME="${DATABASE_URL##*/}"
DATABASE_NAME="${DATABASE_NAME%%\?*}"

docker exec paygate-postgres-test psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
  -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DATABASE_NAME}' AND pid <> pg_backend_pid();" >/dev/null
docker exec paygate-postgres-test psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS ${DATABASE_NAME};" >/dev/null
docker exec paygate-postgres-test psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
  -c "CREATE DATABASE ${DATABASE_NAME} OWNER paygate;" >/dev/null

MIGRATE_BIN="$(command -v migrate || true)"
if [[ -z "${MIGRATE_BIN}" ]]; then
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
  MIGRATE_BIN="$(go env GOPATH)/bin/migrate"
fi
"${MIGRATE_BIN}" -path ./migrations -database "${DATABASE_URL}" up >/dev/null
