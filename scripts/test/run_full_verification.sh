#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_FUZZ_SMOKE="${RUN_FUZZ_SMOKE:-true}"
RUN_LOAD_SMOKE="${RUN_LOAD_SMOKE:-false}"
RUN_CHAOS="${RUN_CHAOS:-false}"

cd "${ROOT_DIR}"

go test ./...
go vet ./...
go test -tags=integration ./tests/integration/...

if [[ "${RUN_FUZZ_SMOKE}" == "true" ]]; then
  go test ./internal/merchant -run=^$ -fuzz=FuzzSessionManagerParse -fuzztime=3s
  go test ./internal/payout -run=^$ -fuzz=FuzzVerifyRailPayload -fuzztime=3s
  go test ./internal/eventschema -run=^$ -fuzz=FuzzValidateDocument -fuzztime=3s
fi

(
  cd dashboard
  pnpm lint
  pnpm build
  pnpm test:e2e
)

if [[ "${RUN_LOAD_SMOKE}" == "true" ]]; then
  START_API=true "${ROOT_DIR}/scripts/test/run_load_smoke.sh"
fi

if [[ "${RUN_CHAOS}" == "true" ]]; then
  START_API=true "${ROOT_DIR}/scripts/test/run_chaos_suite.sh"
fi
