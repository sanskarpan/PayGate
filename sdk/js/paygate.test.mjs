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

test("client can create card tokens and webhook subscriptions", async () => {
  const paths = [];
  const client = createClient({
    baseUrl: "http://example.test",
    keyId: "key",
    keySecret: "secret",
    fetchImpl: async (url, init) => {
      paths.push(new URL(url).pathname);
      if (url.endsWith("/v1/card-tokens")) {
        return new Response(JSON.stringify({ id: "ctok_1", brand: "visa", last4: "1111", token_type: "single_use", reusable: false }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (url.endsWith("/v1/webhooks")) {
        return new Response(JSON.stringify({ id: "wh_1", url: "https://example.test/hook", status: "active", signature_mode: "compat" }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      throw new Error(`unexpected request ${url}`);
    },
  });

  const token = await client.createCardToken({ card_number: "4111111111111111", exp_month: 12, exp_year: 2030 });
  assert.equal(token.id, "ctok_1");
  const webhook = await client.createWebhookSubscription({ url: "https://example.test/hook", events: ["payment.captured"] });
  assert.equal(webhook.id, "wh_1");
  assert.deepEqual(paths, ["/v1/card-tokens", "/v1/webhooks"]);
});
