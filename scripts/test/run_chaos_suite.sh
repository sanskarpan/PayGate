#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_PORT="${API_PORT:-38091}"
API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:${API_PORT}}"
TOXIPROXY_NAME="${TOXIPROXY_NAME:-paygate-toxiproxy}"
TOXIPROXY_API="${TOXIPROXY_API:-http://127.0.0.1:8474}"
START_API="${START_API:-false}"
API_PID=""
API_LOG_FILE="${API_LOG_FILE:-/tmp/paygate-chaos-api.log}"

wait_for_api() {
  local attempts="${1:-60}"
  local delay_seconds="${2:-1}"

  for ((i = 0; i < attempts; i++)); do
    if curl -fsS "${API_BASE_URL}/readyz" >/dev/null 2>&1; then
      return 0
    fi
    if [[ -n "${API_PID}" ]] && ! kill -0 "${API_PID}" >/dev/null 2>&1; then
      echo "api-gateway exited before becoming ready" >&2
      if [[ -f "${API_LOG_FILE}" ]]; then
        cat "${API_LOG_FILE}" >&2
      fi
      return 1
    fi
    sleep "${delay_seconds}"
  done

  echo "timed out waiting for api-gateway readiness at ${API_BASE_URL}/readyz" >&2
  if [[ -f "${API_LOG_FILE}" ]]; then
    cat "${API_LOG_FILE}" >&2
  fi
  return 1
}

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "${API_PID}" >/dev/null 2>&1; then
    kill "${API_PID}" >/dev/null 2>&1 || true
    wait "${API_PID}" 2>/dev/null || true
  fi
  docker rm -f "${TOXIPROXY_NAME}" >/dev/null 2>&1 || true
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

"${ROOT_DIR}/scripts/test/prepare_local_stack.sh"

docker rm -f "${TOXIPROXY_NAME}" >/dev/null 2>&1 || true
docker run -d --name "${TOXIPROXY_NAME}" \
  -p 8474:8474 \
  -p 26379:26379 \
  -p 29092:29092 \
  ghcr.io/shopify/toxiproxy >/dev/null

until curl -fsS "${TOXIPROXY_API}/version" >/dev/null 2>&1; do
  sleep 1
done

ensure_proxy() {
  local name="$1"
  local listen="$2"
  local upstream="$3"

  if curl -fsS "${TOXIPROXY_API}/proxies/${name}" >/dev/null 2>&1; then
    curl -fsS -X POST "${TOXIPROXY_API}/proxies/${name}" \
      -H 'Content-Type: application/json' \
      --data "$(jq -n --arg listen "${listen}" --arg upstream "${upstream}" '{listen: $listen, upstream: $upstream, enabled: true}')" >/dev/null
    return
  fi

  curl -fsS -X POST "${TOXIPROXY_API}/proxies" \
    -H 'Content-Type: application/json' \
    --data "$(jq -n --arg name "${name}" --arg listen "${listen}" --arg upstream "${upstream}" '{name: $name, listen: $listen, upstream: $upstream, enabled: true}')" >/dev/null
}

ensure_proxy redis 0.0.0.0:26379 127.0.0.1:6380
ensure_proxy kafka 0.0.0.0:29092 127.0.0.1:9092

if [[ "${START_API}" == "true" ]]; then
  env \
    PORT="${API_PORT}" \
    DATABASE_URL="${DATABASE_URL:-postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable}" \
    REDIS_ADDR=127.0.0.1:26379 \
    KAFKA_BROKERS=127.0.0.1:29092 \
    OTEL_EXPORTER_STDOUT=false \
    go run ./cmd/api-gateway >"${API_LOG_FILE}" 2>&1 &
  API_PID=$!
  wait_for_api
fi

if [[ -z "${CHAOS_API_KEY_ID:-}" || -z "${CHAOS_API_KEY_SECRET:-}" ]]; then
  eval "$(API_BASE_URL="${API_BASE_URL}" BOOTSTRAP_KEY_SCOPE=admin "${ROOT_DIR}/scripts/test/bootstrap_test_merchant.sh")"
  export CHAOS_API_KEY_ID="${API_KEY}"
  export CHAOS_API_KEY_SECRET="${API_SECRET}"
  export CHAOS_AUTH_HEADER="${AUTH_HEADER}"
fi
export CHAOS_API_BASE_URL="${API_BASE_URL}"

go test -tags=chaos ./tests/chaos/...
