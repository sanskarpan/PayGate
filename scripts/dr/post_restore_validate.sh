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
  auto_capture: false,
  metadata: { drill: "post_restore_validate" }
}')"

token_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/card-tokens" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H 'Content-Type: application/json' \
    --data '{"card_number":"4111111111111111","exp_month":12,"exp_year":2030,"cardholder_name":"Restore Validation","reusable":false}'
)"
token_id="$(jq -r '.id' <<<"${token_json}")"
if [[ -z "${token_id}" || "${token_id}" == "null" ]]; then
  echo "failed to mint card token for restore validation" >&2
  exit 1
fi

payment_payload="$(jq --arg token_id "${token_id}" '.payment_method_token_id = $token_id' <<<"${payment_payload}")"

payment_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/payments/authorize" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H 'Content-Type: application/json' \
    --data "${payment_payload}"
)"
payment_status="$(jq -r '.status' <<<"${payment_json}")"
payment_id="$(jq -r '.id' <<<"${payment_json}")"
if [[ "${payment_status}" != "authorized" && "${payment_status}" != "requires_action" && "${payment_status}" != "captured" ]]; then
  echo "unexpected payment status: ${payment_status}" >&2
  exit 1
fi

if [[ "${payment_status}" == "authorized" ]]; then
  capture_json="$(
    curl -fsS -X POST "${API_BASE_URL}/v1/payments/${payment_id}/capture" \
      -H "Authorization: ${AUTH_HEADER}" \
      -H 'Content-Type: application/json' \
      --data '{"amount":4200}'
  )"
  capture_status="$(jq -r '.status' <<<"${capture_json}")"
  if [[ "${capture_status}" != "captured" ]]; then
    echo "unexpected capture status: ${capture_status}" >&2
    exit 1
  fi
fi

settlement_json="$(
  curl -fsS -X POST "${API_BASE_URL}/v1/settlements/batch" \
    -H "Authorization: ${AUTH_HEADER}" \
    -H 'Content-Type: application/json' \
    --data '{}'
)"
echo "${settlement_json}" | jq .

recon_json="$(curl -fsS -H "Authorization: ${AUTH_HEADER}" "${API_BASE_URL}/v1/recon/mismatches")"
recon_count="$(jq -r '.count // 0' <<<"${recon_json}")"
if [[ "${recon_count}" != "0" ]]; then
  echo "expected zero recon mismatches after restore validation, got ${recon_count}" >&2
  exit 1
fi

for _ in {1..15}; do
  outbox_unpublished="$(curl -fsS "${API_BASE_URL}/readyz" | jq -r '.checks.outbox_unpublished')"
  if [[ "${outbox_unpublished}" == "0" ]]; then
    break
  fi
  sleep 2
done

if [[ "${outbox_unpublished:-}" != "0" ]]; then
  echo "outbox backlog did not drain after restore validation: ${outbox_unpublished}" >&2
  exit 1
fi

echo "post-restore validation passed"
