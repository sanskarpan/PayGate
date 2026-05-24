# Integration Recipes

## Sandbox Bootstrap

Use the Go CLI to create a test merchant and admin key:

```bash
go run ./cmd/sandbox-bootstrap -base-url http://127.0.0.1:8090
```

The command prints:

- `MERCHANT_ID`
- `API_KEY`
- `API_SECRET`
- `AUTH_HEADER`

## Postman Collection

The repo ships a starter Postman collection at:

- `postman/PayGate.postman_collection.json`

Additional curated collections:

- `bruno/paygate`
- `insomnia/PayGate.json`

Import it, then populate:

- `merchant_id`
- `auth_header`
- `order_id`
- `payment_id`

## SDKs And Reference Apps

The repo also ships:

- Go SDK: `sdk/go/paygate`
- JavaScript SDK: `sdk/js`
- Server-side Go example: `examples/server-only-go`
- Hosted checkout Node example: `examples/hosted-checkout-node`
- Webhook consumer example: `examples/webhook-consumer`

Verify these artifacts with:

```bash
./scripts/test/verify_integrations_artifacts.sh
```

## Post-Restore Validation

After a restore or local recovery event, run:

```bash
./scripts/dr/post_restore_validate.sh
```

The script verifies:

- readiness endpoint response
- merchant bootstrap
- idempotent order replay
- payment creation after restore
