# PayGate JavaScript SDK

Minimal official JavaScript client for browser or Node sandbox integrations.

Current helpers:
- order creation
- payment create/capture
- refund create
- webhook HMAC verification

Example:

```js
import { createClient } from "@paygate/sdk";

const client = createClient({
  baseUrl: "http://127.0.0.1:8090",
  keyId: process.env.PAYGATE_KEY_ID,
  keySecret: process.env.PAYGATE_KEY_SECRET,
});

const order = await client.createOrder({
  amount: 4200,
  currency: "INR",
  receipt: "order-1",
}, "idem-order-1");
```
