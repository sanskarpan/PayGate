#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEFAULT_DATABASE_URL="postgres://pay""gate:pay""gate@localhost:5435/paygate?sslmode=disable"
DATABASE_URL="${DATABASE_URL:-${DEFAULT_DATABASE_URL}}"
RESET_DATABASE="${RESET_DATABASE:-true}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-paygate-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-paygate-redis}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-paygate-kafka}"
DATABASE_NAME="${DATABASE_NAME:-${DATABASE_URL##*/}}"
DATABASE_NAME="${DATABASE_NAME%%\?*}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose is required" >&2
  exit 1
fi

cd "${ROOT_DIR}"

docker compose up -d postgres redis kafka >/dev/null

until docker exec "${POSTGRES_CONTAINER}" pg_isready -U paygate -d postgres >/dev/null 2>&1; do
  sleep 2
done

until docker exec "${REDIS_CONTAINER}" redis-cli ping >/dev/null 2>&1; do
  sleep 2
done

until docker exec "${KAFKA_CONTAINER}" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do
  sleep 2
done

if [[ "${RESET_DATABASE}" == "true" ]]; then
  docker exec "${POSTGRES_CONTAINER}" psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
    -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DATABASE_NAME}' AND pid <> pg_backend_pid();" >/dev/null
  docker exec "${POSTGRES_CONTAINER}" psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS ${DATABASE_NAME};" >/dev/null
  docker exec "${POSTGRES_CONTAINER}" psql -U paygate -d postgres -v ON_ERROR_STOP=1 \
    -c "CREATE DATABASE ${DATABASE_NAME} OWNER paygate;" >/dev/null
fi

MIGRATE_BIN="$(command -v migrate || true)"
if [[ -z "${MIGRATE_BIN}" ]]; then
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
  MIGRATE_BIN="$(go env GOPATH)/bin/migrate"
fi
"${MIGRATE_BIN}" -path ./migrations -database "${DATABASE_URL}" up
