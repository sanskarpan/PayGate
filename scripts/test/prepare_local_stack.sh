#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable}"

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

until docker exec paygate-postgres pg_isready -U paygate -d paygate >/dev/null 2>&1; do
  sleep 2
done

until docker exec paygate-redis redis-cli ping >/dev/null 2>&1; do
  sleep 2
done

until docker exec paygate-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do
  sleep 2
done

MIGRATE_BIN="$(command -v migrate || true)"
if [[ -z "${MIGRATE_BIN}" ]]; then
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
  MIGRATE_BIN="$(go env GOPATH)/bin/migrate"
fi

MIGRATE_OUTPUT="$("${MIGRATE_BIN}" -path ./migrations -database "${DATABASE_URL}" up 2>&1 || true)"
if [[ -n "${MIGRATE_OUTPUT}" && "${MIGRATE_OUTPUT}" != *"no change"* ]]; then
  echo "${MIGRATE_OUTPUT}" >&2
  exit 1
fi
