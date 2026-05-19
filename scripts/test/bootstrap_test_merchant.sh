#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8090}"
SUFFIX="${BOOTSTRAP_SUFFIX:-$(date +%s)-$$}"
KEY_SCOPE="${BOOTSTRAP_KEY_SCOPE:-admin}"

merchant_payload="$(jq -n --arg suffix "${SUFFIX}" '{
  name: ("Test Merchant " + $suffix),
  email: ("merchant+" + $suffix + "@tests.paygate.local"),
  business_type: "company",
  settings: { suite: "bootstrap_test_merchant" }
}')"

merchant_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/merchants" \
    -H 'Content-Type: application/json' \
    --data "${merchant_payload}"
)"
merchant_id="$(jq -r '.id' <<<"${merchant_json}")"

key_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/merchants/${merchant_id}/keys" \
    -H 'Content-Type: application/json' \
    --data "$(jq -n --arg scope "${KEY_SCOPE}" '{mode:"test", scope:$scope}')"
)"
api_key="$(jq -r '.key_id' <<<"${key_json}")"
api_secret="$(jq -r '.key_secret' <<<"${key_json}")"
auth_b64="$(printf '%s' "${api_key}:${api_secret}" | base64)"

printf 'MERCHANT_ID=%q\n' "${merchant_id}"
printf 'API_KEY=%q\n' "${api_key}"
printf 'API_SECRET=%q\n' "${api_secret}"
printf 'AUTH_B64=%q\n' "${auth_b64}"
printf 'AUTH_HEADER=%q\n' "Basic ${auth_b64}"
