# Webhook Consumer Example

Webhook consumers should verify:

- `X-PayGate-Signature`
- `Content-Digest`
- `Signature-Input`
- `Signature`

Legacy compatibility:

- existing integrations may continue verifying `X-PayGate-Signature`

Preferred verification:

- validate `Content-Digest` from raw body
- validate structured HTTP Message Signature headers
- reject replayed or stale timestamps in application logic

Reference implementation helpers live in:

- `internal/webhook/deliverer.go`
