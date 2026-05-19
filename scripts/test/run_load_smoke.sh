#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_PORT="${API_PORT:-38090}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${API_PORT}}"
LOAD_SCRIPT="${LOAD_SCRIPT:-tests/load/ci_smoke.js}"
START_API="${START_API:-false}"
API_PID=""

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "${API_PID}" >/dev/null 2>&1; then
    kill "${API_PID}" >/dev/null 2>&1 || true
    wait "${API_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

cd "${ROOT_DIR}"

if [[ "${START_API}" == "true" ]]; then
  scripts/dr/recreate_local_env.sh
  env \
    PORT="${API_PORT}" \
    DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable}" \
    REDIS_ADDR="${REDIS_ADDR:-localhost:6380}" \
    KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}" \
    OTEL_EXPORTER_STDOUT=false \
    go run ./cmd/api-gateway >/tmp/paygate-load-api.log 2>&1 &
  API_PID=$!
  until curl -fsS "${BASE_URL}/readyz" >/dev/null; do
    sleep 2
  done
fi

if [[ -z "${API_KEY:-}" || -z "${API_SECRET:-}" ]]; then
  eval "$(API_BASE_URL="${BASE_URL}" BOOTSTRAP_KEY_SCOPE=write "${ROOT_DIR}/scripts/test/bootstrap_test_merchant.sh")"
fi

K6_BASE_URL="${K6_BASE_URL:-${BASE_URL/127.0.0.1/host.docker.internal}}"
K6_BASE_URL="${K6_BASE_URL/localhost/host.docker.internal}"

docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -v "${ROOT_DIR}:/work" \
  -w /work \
  grafana/k6:0.49.0 run \
  --env BASE_URL="${K6_BASE_URL}" \
  --env API_KEY="${API_KEY}" \
  --env API_SECRET="${API_SECRET}" \
  "${LOAD_SCRIPT}"
