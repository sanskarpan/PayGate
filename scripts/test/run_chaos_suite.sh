#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_PORT="${API_PORT:-38091}"
API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:${API_PORT}}"
TOXIPROXY_NAME="${TOXIPROXY_NAME:-paygate-toxiproxy}"
TOXIPROXY_API="${TOXIPROXY_API:-http://127.0.0.1:8474}"
START_API="${START_API:-false}"
API_PID=""

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

scripts/dr/recreate_local_env.sh

docker rm -f "${TOXIPROXY_NAME}" >/dev/null 2>&1 || true
docker run -d --name "${TOXIPROXY_NAME}" \
  -p 8474:8474 \
  -p 26379:26379 \
  -p 29092:29092 \
  ghcr.io/shopify/toxiproxy >/dev/null

until curl -fsS "${TOXIPROXY_API}/version" >/dev/null; do
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
    go run ./cmd/api-gateway >/tmp/paygate-chaos-api.log 2>&1 &
  API_PID=$!
  until curl -fsS "${API_BASE_URL}/readyz" >/dev/null; do
    sleep 2
  done
fi

if [[ -z "${CHAOS_API_KEY_ID:-}" || -z "${CHAOS_API_KEY_SECRET:-}" ]]; then
  eval "$(API_BASE_URL="${API_BASE_URL}" BOOTSTRAP_KEY_SCOPE=write "${ROOT_DIR}/scripts/test/bootstrap_test_merchant.sh")"
  export CHAOS_API_KEY_ID="${API_KEY}"
  export CHAOS_API_KEY_SECRET="${API_SECRET}"
  export CHAOS_AUTH_HEADER="${AUTH_HEADER}"
fi
export CHAOS_API_BASE_URL="${API_BASE_URL}"

go test -tags=chaos ./tests/chaos/...
