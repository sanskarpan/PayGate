#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5433/paygate_test?sslmode=disable}"

cd "${ROOT_DIR}"

docker compose -f docker-compose.test.yml up -d postgres-test redis-test >/dev/null

# Wait for a service to answer a real command, not just a liveness probe.
#
# pg_isready succeeds against the TEMPORARY server that initdb runs on a first
# boot. That server then shuts down so the real one can start, so a single
# successful probe is not enough: the next psql call fails with
# "the database system is shutting down". Require several consecutive
# successes, and bound the wait so CI fails loudly instead of hanging.
wait_for() {
  local name="$1" required="$2" attempts="$3"
  shift 3
  local streak=0
  for _ in $(seq 1 "${attempts}"); do
    if "$@" >/dev/null 2>&1; then
      streak=$((streak + 1))
      if [[ "${streak}" -ge "${required}" ]]; then
        return 0
      fi
    else
      streak=0
    fi
    sleep 1
  done
  echo "timed out waiting for ${name} to become ready" >&2
  return 1
}

wait_for "postgres-test" 5 120 docker exec paygate-postgres-test psql -U paygate -d postgres -c 'SELECT 1'
wait_for "redis-test" 3 60 docker exec paygate-redis-test redis-cli ping

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
