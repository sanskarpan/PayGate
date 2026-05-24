#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8090}"

ready="$(curl -fsS "${API_BASE_URL}/readyz")"
echo "${ready}" | jq .

eval "$("$(dirname "$0")"/../test/bootstrap_test_merchant.sh)"

order_payload="$(jq -n '{
  amount: 4200,
  currency: "INR",
  receipt: "restore-check-order",
  notes: { drill: "post_restore_validate" }
}')"

order_one="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/orders" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H "Idempotency-Key: restore-validate-order" \
    -H 'Content-Type: application/json' \
    --data "${order_payload}"
)"
order_two="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/orders" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H "Idempotency-Key: restore-validate-order" \
    -H 'Content-Type: application/json' \
    --data "${order_payload}"
)"

order_id="$(jq -r '.id' <<<"${order_one}")"
replayed_id="$(jq -r '.id' <<<"${order_two}")"
if [[ "${order_id}" != "${replayed_id}" ]]; then
  echo "idempotent replay failed: ${order_id} != ${replayed_id}" >&2
  exit 1
fi

payment_payload="$(jq -n --arg order_id "${order_id}" '{
  order_id: $order_id,
  method: "card",
  payment_method_token_id: "tok_sandbox_single_use",
  capture: true,
  metadata: { drill: "post_restore_validate" }
}')"

payment_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/payments" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H 'Content-Type: application/json' \
    --data "${payment_payload}"
)"
payment_status="$(jq -r '.status' <<<"${payment_json}")"
if [[ "${payment_status}" != "captured" && "${payment_status}" != "authorized" ]]; then
  echo "unexpected payment status: ${payment_status}" >&2
  exit 1
fi

echo "post-restore validation passed"
