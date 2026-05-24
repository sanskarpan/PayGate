#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "${ROOT_DIR}"

go test ./sdk/go/paygate
node --test sdk/js/paygate.test.mjs

(
  cd examples/server-only-go
  go build ./...
)

node --check examples/hosted-checkout-node/index.mjs
node --check examples/webhook-consumer/server.mjs

jq empty postman/PayGate.postman_collection.json
jq empty insomnia/PayGate.json
jq empty bruno/paygate/bruno.json
