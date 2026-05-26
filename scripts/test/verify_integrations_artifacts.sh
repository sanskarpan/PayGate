#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "${ROOT_DIR}"

go test ./sdk/go/paygate
node --test sdk/js/paygate.test.mjs
go test -count=1 -tags=integration ./tests/integration/... -run 'TestIntegrationSDKClientsAgainstLiveMux'

(
  cd examples/server-only-go
  go build ./...
)

go run ./cmd/sandbox-bootstrap -h >/dev/null 2>&1 || true
node --check examples/hosted-checkout-node/index.mjs
node --check examples/webhook-consumer/server.mjs

jq empty postman/PayGate.postman_collection.json
jq empty insomnia/PayGate.json
jq empty bruno/paygate/bruno.json

jq -e '.item[] | .. | objects | select(.request? and (.request.url.raw // "" | contains("/v1/orders")))' postman/PayGate.postman_collection.json >/dev/null
jq -e '.item[] | .. | objects | select(.request? and (.request.url.raw // "" | contains("/v1/payments/authorize")))' postman/PayGate.postman_collection.json >/dev/null
jq -e '.item[] | .. | objects | select(.request? and (.request.url.raw // "" | contains("/v1/webhooks")))' postman/PayGate.postman_collection.json >/dev/null
