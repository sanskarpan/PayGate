# PayGate Go SDK

Minimal official Go client for server-side PayGate integrations.

Current helpers:
- order creation
- payment create/capture
- refund create
- webhook HMAC verification

Example:

```go
client := paygate.New("http://127.0.0.1:8090", os.Getenv("PAYGATE_KEY_ID"), os.Getenv("PAYGATE_KEY_SECRET"))
order, err := client.CreateOrder(ctx, map[string]any{
  "amount":   4200,
  "currency": "INR",
  "receipt":  "order-1",
}, "idem-order-1")
```
