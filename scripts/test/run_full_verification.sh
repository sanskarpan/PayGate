#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_FUZZ_SMOKE="${RUN_FUZZ_SMOKE:-true}"
RUN_LOAD_SMOKE="${RUN_LOAD_SMOKE:-false}"
RUN_CHAOS="${RUN_CHAOS:-false}"
FUZZ_GOMAXPROCS="${FUZZ_GOMAXPROCS:-2}"
FUZZ_TIMEOUT="${FUZZ_TIMEOUT:-30s}"

cd "${ROOT_DIR}"

go test ./...
go vet ./...
go test -tags=integration ./tests/integration/...

if [[ "${RUN_FUZZ_SMOKE}" == "true" ]]; then
  GOMAXPROCS="${FUZZ_GOMAXPROCS}" go test ./internal/merchant -run=^$ -fuzz=FuzzSessionManagerParse -fuzztime=3s -timeout="${FUZZ_TIMEOUT}"
  GOMAXPROCS="${FUZZ_GOMAXPROCS}" go test ./internal/payout -run=^$ -fuzz=FuzzVerifyRailPayload -fuzztime=3s -timeout="${FUZZ_TIMEOUT}"
  GOMAXPROCS="${FUZZ_GOMAXPROCS}" go test ./internal/eventschema -run=^$ -fuzz=FuzzValidateDocument -fuzztime=3s -timeout="${FUZZ_TIMEOUT}"
fi

(
  cd dashboard
  pnpm lint
  pnpm build
  pnpm test:e2e
)

"${ROOT_DIR}/scripts/test/verify_integrations_artifacts.sh"

if [[ "${RUN_LOAD_SMOKE}" == "true" ]]; then
  START_API=true "${ROOT_DIR}/scripts/test/run_load_smoke.sh"
fi

if [[ "${RUN_CHAOS}" == "true" ]]; then
  START_API=true "${ROOT_DIR}/scripts/test/run_chaos_suite.sh"
fi
