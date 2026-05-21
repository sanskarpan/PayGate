import { expect, test, request as playwrightRequest } from "@playwright/test";

import { loadSeedData, storageStatePath } from "./helpers/seed";

function basicAuthHeader(keyID: string, keySecret: string) {
  return `Basic ${Buffer.from(`${keyID}:${keySecret}`).toString("base64")}`;
}

async function poll<T>(
  fn: () => Promise<T>,
  done: (value: T) => boolean,
  timeoutMs = 60_000,
  intervalMs = 1_000,
) {
  const started = Date.now();
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const value = await fn();
    if (done(value)) {
      return value;
    }
    if (Date.now() - started >= timeoutMs) {
      throw new Error("timed out waiting for expected async state");
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}

test("backend smoke covers idempotency, money flow, async delivery, and dashboard session health", async ({ request, baseURL }) => {
  const seed = await loadSeedData();
  const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:38090";
  const authHeader = { Authorization: basicAuthHeader(seed.adminKeyID, seed.adminKeySecret) };

  const ready = await request.get(`${apiBaseUrl}/readyz`);
  expect(ready.ok()).toBeTruthy();
  const readyBody = await ready.json();
  expect(readyBody.checks.postgres).toBe("ok");
  expect(["ok", "not_configured"]).toContain(readyBody.checks.redis);
  expect(Number(readyBody.checks.outbox_unpublished)).toBeGreaterThanOrEqual(0);

  const invalidWebhook = await request.post(`${apiBaseUrl}/v1/webhooks`, {
    headers: authHeader,
    data: {
      url: "http://example.com/insecure",
      events: ["payment.captured"],
    },
  });
  expect(invalidWebhook.status()).toBe(400);

  const idempotencyKey = `pw-smoke-${Date.now()}`;
  const orderPayload = {
    amount: 4200,
    currency: "INR",
    receipt: `pw-smoke-${Date.now()}`,
    notes: { suite: "playwright", path: "idempotency" },
  };
  const firstOrder = await request.post(`${apiBaseUrl}/v1/orders`, {
    headers: { ...authHeader, "Idempotency-Key": idempotencyKey },
    data: orderPayload,
  });
  expect(firstOrder.status()).toBe(201);
  const secondOrder = await request.post(`${apiBaseUrl}/v1/orders`, {
    headers: { ...authHeader, "Idempotency-Key": idempotencyKey },
    data: orderPayload,
  });
  expect(secondOrder.status()).toBe(201);
  expect(secondOrder.headers()["idempotent-replayed"]).toBe("true");
  const firstOrderBody = await firstOrder.json();
  const secondOrderBody = await secondOrder.json();
  expect(secondOrderBody.id).toBe(firstOrderBody.id);

  const payment = await request.get(`${apiBaseUrl}/v1/payments/${seed.paymentID}`, { headers: authHeader });
  expect(payment.ok()).toBeTruthy();
  const paymentBody = await payment.json();
  expect(paymentBody.status).toBe("captured");
  expect(paymentBody.order_id).toBe(seed.orderID);

  const refunds = await request.get(`${apiBaseUrl}/v1/payments/${seed.paymentID}/refunds`, { headers: authHeader });
  expect(refunds.ok()).toBeTruthy();
  const refundsBody = await refunds.json();
  expect(refundsBody.items.some((item: { id: string; status: string }) => item.id === seed.refundID && item.status === "processed")).toBeTruthy();

  const settlement = await request.get(`${apiBaseUrl}/v1/settlements/${seed.settlementID}`, { headers: authHeader });
  expect(settlement.ok()).toBeTruthy();
  const settlementBody = await settlement.json();
  expect(settlementBody.items.length).toBeGreaterThanOrEqual(2);

  const payout = await request.get(`${apiBaseUrl}/v1/payouts/${seed.payoutID}`, { headers: authHeader });
  expect(payout.ok()).toBeTruthy();
  const payoutBody = await payout.json();
  expect(payoutBody.status).toBe("completed");
  expect(Object.prototype.hasOwnProperty.call(payoutBody, "saga_id")).toBeTruthy();
  expect(typeof payoutBody.saga_id).toBe("string");

  const dispute = await request.get(`${apiBaseUrl}/v1/disputes/${seed.disputeID}`, { headers: authHeader });
  expect(dispute.ok()).toBeTruthy();
  const disputeBody = await dispute.json();
  expect(disputeBody.status).toBe("won");
  expect(disputeBody.settlement_id).toBe(seed.settlementID);

  const deliveriesBody = await poll(
    async () => {
      const deliveries = await request.get(`${apiBaseUrl}/v1/webhooks/${seed.webhookID}/deliveries`, { headers: authHeader });
      expect(deliveries.ok()).toBeTruthy();
      return deliveries.json();
    },
    (body: any) =>
      Array.isArray(body.items) &&
      body.items.some((item: { event_type?: string; status?: string }) => item.event_type === "payment.captured" && item.status === "succeeded"),
  );
  expect(deliveriesBody.items.length).toBeGreaterThanOrEqual(1);

  const sinkBody = await poll(
    async () => {
      const sink = await request.get(`${baseURL}/api/test-webhook?token=${encodeURIComponent(seed.webhookToken)}`);
      expect(sink.ok()).toBeTruthy();
      return sink.json();
    },
    (body: any) => {
      const eventTypes = (body.items ?? [])
        .map((item: { body?: { event_type?: string } }) => item.body?.event_type)
        .filter(Boolean);
      return eventTypes.includes("payment.captured");
    },
  );
  const eventTypes = sinkBody.items.map((item: { body?: { event_type?: string } }) => item.body?.event_type).filter(Boolean);
  expect(eventTypes).toContain("payment.captured");

  const sessionContext = await playwrightRequest.newContext({
    baseURL: apiBaseUrl,
    storageState: storageStatePath,
  });
  const viewer = await sessionContext.get("/v1/dashboard/me");
  expect(viewer.ok()).toBeTruthy();
  const viewerBody = await viewer.json();
  expect(viewerBody.merchant_id).toBe(seed.merchantID);
  await sessionContext.dispose();
});
