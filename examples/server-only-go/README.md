# Server-Only API Example

This reference app pattern is for backend-only integrations that:

1. create orders server-side
2. authorize or capture payments
3. create refunds
4. request reports

Recommended helpers:

- `go run ./cmd/sandbox-bootstrap`
- `postman/PayGate.postman_collection.json`

Suggested implementation flow:

- create `Authorization: Basic <base64(key:secret)>`
- include idempotency keys on mutating requests
- verify webhook signatures using both legacy HMAC and structured HTTP Message Signatures
