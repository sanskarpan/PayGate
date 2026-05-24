import test from "node:test";
import assert from "node:assert/strict";
import { createClient, verifyWebhookSignature, VERSION } from "./paygate.js";

test("createClient sets auth and idempotency headers", async () => {
  let seenAuth = "";
  let seenIdempotency = "";
  const client = createClient({
    baseUrl: "http://example.test",
    keyId: "key",
    keySecret: "secret",
    fetchImpl: async (url, init) => {
      seenAuth = init.headers.get("Authorization");
      seenIdempotency = init.headers.get("Idempotency-Key");
      return new Response(JSON.stringify({ id: "order_1" }), { status: 200, headers: { "Content-Type": "application/json" } });
    },
  });
  const order = await client.createOrder({ amount: 4200, currency: "INR" }, "idem-1");
  assert.equal(order.id, "order_1");
  assert.match(seenAuth, /^Basic /);
  assert.equal(seenIdempotency, "idem-1");
  assert.equal(client.version, VERSION);
});

test("verifyWebhookSignature verifies hmac", async () => {
  assert.equal(await verifyWebhookSignature({
    secret: "secret",
    bodyText: '{"id":"evt_1"}',
    signature: "16abb10adb33ff9cff34f6a57fc2c0b902c11ea19fe73dae86f2940c235e7ed5",
  }), true);
});
