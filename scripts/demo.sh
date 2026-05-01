#!/usr/bin/env bash
# demo.sh — end-to-end PayGate API walkthrough
#
# Prerequisites:
#   - API gateway running at BASE_URL (default: http://localhost:8080)
#   - curl + jq installed
#
# Usage:
#   ./scripts/demo.sh
#   BASE_URL=http://localhost:8080 ./scripts/demo.sh

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

step() { echo -e "\n${BOLD}${CYAN}▶ $*${NC}"; }
ok()   { echo -e "${GREEN}✓ $*${NC}"; }
fail() { echo -e "${RED}✗ $*${NC}"; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || fail "'$1' is required but not installed"
}

require curl
require jq

step "1 — Health check"
STATUS=$(curl -sf "${BASE_URL}/healthz" | jq -r '.status')
[[ "$STATUS" == "ok" ]] || fail "gateway not healthy: $STATUS"
ok "gateway healthy"

step "2 — Create merchant"
MERCHANT=$(curl -sf -X POST "${BASE_URL}/v1/merchants" \
  -H "Content-Type: application/json" \
  -d '{"name":"Demo Merchant","email":"demo@example.com","business_type":"company"}')
MERCHANT_ID=$(echo "$MERCHANT" | jq -r '.id')
[[ -n "$MERCHANT_ID" ]] || fail "merchant creation failed"
ok "merchant created: $MERCHANT_ID"

step "3 — Create API key (bootstrap)"
KEY_RESP=$(curl -sf -X POST "${BASE_URL}/v1/merchants/${MERCHANT_ID}/keys" \
  -H "Content-Type: application/json" \
  -d '{"mode":"test","scope":"write"}')
KEY_ID=$(echo "$KEY_RESP" | jq -r '.key_id')
KEY_SECRET=$(echo "$KEY_RESP" | jq -r '.key_secret')
[[ -n "$KEY_ID" ]] || fail "api key creation failed"
ok "api key: $KEY_ID"

AUTH="$(echo -n "${KEY_ID}:${KEY_SECRET}" | base64)"
AUTH_HEADER="Authorization: Basic ${AUTH}"

step "4 — Create order"
ORDER=$(curl -sf -X POST "${BASE_URL}/v1/orders" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -H "Idempotency-Key: demo-order-$(date +%s)" \
  -d '{"amount":9900,"currency":"INR","receipt":"demo-receipt-001"}')
ORDER_ID=$(echo "$ORDER" | jq -r '.id')
[[ -n "$ORDER_ID" ]] || fail "order creation failed"
ok "order created: $ORDER_ID (amount: $(echo "$ORDER" | jq -r '.amount') INR)"

step "5 — Authorize payment"
PAYMENT=$(curl -sf -X POST "${BASE_URL}/v1/payments/authorize" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -H "Idempotency-Key: demo-pay-$(date +%s)" \
  -d "{\"order_id\":\"${ORDER_ID}\",\"amount\":9900,\"currency\":\"INR\",\"method\":\"card\"}")
PAYMENT_ID=$(echo "$PAYMENT" | jq -r '.id')
PAYMENT_STATUS=$(echo "$PAYMENT" | jq -r '.status')
[[ -n "$PAYMENT_ID" ]] || fail "payment authorization failed"
ok "payment authorized: $PAYMENT_ID (status: $PAYMENT_STATUS)"

step "6 — Capture payment"
CAPTURE=$(curl -sf -X POST "${BASE_URL}/v1/payments/${PAYMENT_ID}/capture" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -H "Idempotency-Key: demo-capture-$(date +%s)" \
  -d '{"amount":9900}')
CAPTURE_STATUS=$(echo "$CAPTURE" | jq -r '.status')
ok "payment captured: $PAYMENT_ID (status: $CAPTURE_STATUS)"

step "7 — Fetch ledger balance"
# Read-scoped key needed; reuse write key which has write >= read
BALANCE=$(curl -sf "${BASE_URL}/v1/merchants/me/balance" \
  -H "$AUTH_HEADER")
ok "ledger balance: $(echo "$BALANCE" | jq -c '.balances')"

step "8 — List orders"
ORDERS=$(curl -sf "${BASE_URL}/v1/orders" -H "$AUTH_HEADER")
COUNT=$(echo "$ORDERS" | jq '.count')
ok "orders in account: $COUNT"

echo -e "\n${GREEN}${BOLD}Demo complete.${NC}"
echo "  Merchant:  $MERCHANT_ID"
echo "  Order:     $ORDER_ID"
echo "  Payment:   $PAYMENT_ID"
echo "  Status:    $CAPTURE_STATUS"
