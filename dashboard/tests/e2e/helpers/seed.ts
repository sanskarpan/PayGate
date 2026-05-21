import fs from "node:fs/promises";
import path from "node:path";
import { randomUUID } from "node:crypto";

import type { APIRequestContext } from "@playwright/test";

const appBaseUrl = process.env.APP_BASE_URL || "http://127.0.0.1:33001";
const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:38090";
const authDir = path.resolve(__dirname, "../../../playwright/.auth");

export const storageStatePath = path.join(authDir, "dashboard-user.json");
export const seedStatePath = path.join(authDir, "seed.json");

export type SeedData = {
  merchantID: string;
  adminKeyID: string;
  adminKeySecret: string;
  email: string;
  password: string;
  webhookID: string;
  webhookToken: string;
  orderID: string;
  paymentID: string;
  refundID: string;
  secondOrderID: string;
  secondPaymentID: string;
  settlementID: string;
  payoutID: string;
  disputeID: string;
};

type JsonValue = Record<string, any>;

function basicAuthHeader(keyID: string, keySecret: string) {
  const raw = Buffer.from(`${keyID}:${keySecret}`).toString("base64");
  return `Basic ${raw}`;
}

async function writeSeed(data: SeedData) {
  await fs.mkdir(authDir, { recursive: true });
  await fs.writeFile(seedStatePath, JSON.stringify(data, null, 2));
}

export async function loadSeedData(): Promise<SeedData> {
  const raw = await fs.readFile(seedStatePath, "utf8");
  return JSON.parse(raw) as SeedData;
}

async function appFetch(pathname: string, init?: RequestInit) {
  const response = await fetch(`${appBaseUrl}${pathname}`, init);
  if (!response.ok) {
    throw new Error(`app request failed for ${pathname}: ${response.status}`);
  }
  return response;
}

async function apiJson(
  request: APIRequestContext,
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE",
  pathname: string,
  options?: {
    headers?: Record<string, string>;
    data?: JsonValue;
    expectedStatus?: number;
  },
) {
  const response = await request.fetch(`${apiBaseUrl}${pathname}`, {
    method,
    headers: options?.headers,
    data: options?.data,
  });
  const expectedStatus = options?.expectedStatus;
  if (expectedStatus && response.status() !== expectedStatus) {
    throw new Error(`expected ${expectedStatus} for ${pathname}, got ${response.status()}: ${await response.text()}`);
  }
  if (!response.ok()) {
    throw new Error(`request failed for ${pathname}: ${response.status()} ${await response.text()}`);
  }
  return response.json();
}

async function poll<T>(
  label: string,
  fn: () => Promise<T>,
  done: (value: T) => boolean,
  timeoutMs = 30_000,
  intervalMs = 500,
) {
  const started = Date.now();
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const value = await fn();
    if (done(value)) {
      return value;
    }
    if (Date.now() - started >= timeoutMs) {
      throw new Error(`timed out waiting for ${label}`);
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}

export async function ensureSeedData(request: APIRequestContext): Promise<SeedData> {
  const suffix = randomUUID().slice(0, 8);
  const email = `owner+${suffix}@playwright.test`;
  const password = `Playwright!${suffix}Ready`;
  const webhookToken = `token-${suffix}`;

  await appFetch(`/api/test-webhook?token=${encodeURIComponent(webhookToken)}`, { method: "DELETE" });

  const merchant = await apiJson(request, "POST", "/v1/merchants", {
    expectedStatus: 201,
    data: {
      name: `Playwright Merchant ${suffix}`,
      email: `merchant+${suffix}@playwright.test`,
      business_type: "company",
      settings: { suite: "playwright-e2e" },
    },
  });
  const merchantID = merchant.id as string;

  const adminKey = await apiJson(request, "POST", `/v1/merchants/${merchantID}/keys`, {
    expectedStatus: 201,
    data: { mode: "test", scope: "admin" },
  });
  const adminKeyID = adminKey.key_id as string;
  const adminKeySecret = adminKey.key_secret as string;
  const authHeader = { Authorization: basicAuthHeader(adminKeyID, adminKeySecret) };

  await apiJson(request, "POST", `/v1/merchants/${merchantID}/users/bootstrap`, {
    expectedStatus: 201,
    data: { email, password },
  });

  const webhook = await apiJson(request, "POST", "/v1/webhooks", {
    expectedStatus: 201,
    headers: authHeader,
    data: {
      url: `${appBaseUrl}/api/test-webhook?token=${encodeURIComponent(webhookToken)}`,
      events: ["payment.captured", "dispute.won"],
    },
  });
  const webhookID = webhook.id as string;

  await apiJson(request, "POST", "/v1/gateway/scenarios", {
    expectedStatus: 201,
    headers: authHeader,
    data: {
      merchant_id: merchantID,
      mode: "success",
      failure_rate: 0,
      delay_ms: 0,
    },
  });

  // The API process reports ready before the async relay/consumer pair has
  // fully settled. Give the webhook worker a short warm-up window so the first
  // capture events are not produced before the consumer is subscribed.
  await new Promise((resolve) => setTimeout(resolve, 2500));

  const order = await apiJson(request, "POST", "/v1/orders", {
    expectedStatus: 201,
    headers: { ...authHeader, "Idempotency-Key": `order-primary-${suffix}` },
    data: {
      amount: 10000,
      currency: "INR",
      receipt: `pw-primary-${suffix}`,
      notes: { suite: "playwright", flow: "refund" },
    },
  });
  const orderID = order.id as string;

  const authorized = await apiJson(request, "POST", "/v1/payments/authorize", {
    expectedStatus: 201,
    headers: { ...authHeader, "Idempotency-Key": `payment-primary-${suffix}` },
    data: {
      order_id: orderID,
      amount: 10000,
      currency: "INR",
      method: "card",
      auto_capture: false,
    },
  });
  const paymentID = authorized.id as string;

  await apiJson(request, "POST", `/v1/payments/${paymentID}/capture`, {
    expectedStatus: 200,
    headers: authHeader,
    data: { amount: 10000 },
  });

  const refund = await apiJson(request, "POST", `/v1/payments/${paymentID}/refunds`, {
    expectedStatus: 201,
    headers: authHeader,
    data: {
      amount: 1500,
      reason: "requested_by_customer",
      notes: { suite: "playwright" },
    },
  });
  const refundID = refund.id as string;

  const secondOrder = await apiJson(request, "POST", "/v1/orders", {
    expectedStatus: 201,
    headers: { ...authHeader, "Idempotency-Key": `order-secondary-${suffix}` },
    data: {
      amount: 15000,
      currency: "INR",
      receipt: `pw-secondary-${suffix}`,
      notes: { suite: "playwright", flow: "dispute" },
    },
  });
  const secondOrderID = secondOrder.id as string;

  const secondAuthorized = await apiJson(request, "POST", "/v1/payments/authorize", {
    expectedStatus: 201,
    headers: { ...authHeader, "Idempotency-Key": `payment-secondary-${suffix}` },
    data: {
      order_id: secondOrderID,
      amount: 15000,
      currency: "INR",
      method: "card",
      auto_capture: false,
    },
  });
  const secondPaymentID = secondAuthorized.id as string;

  await apiJson(request, "POST", `/v1/payments/${secondPaymentID}/capture`, {
    expectedStatus: 200,
    headers: authHeader,
    data: { amount: 15000 },
  });

  const settlement = await apiJson(request, "POST", "/v1/settlements/batch", {
    expectedStatus: 201,
    headers: authHeader,
    data: {
      period_start: 0,
      period_end: Math.floor(Date.now() / 1000) + 3600,
    },
  });
  const settlementID = settlement.id as string;

  const payout = await apiJson(request, "POST", `/v1/settlements/${settlementID}/payout`, {
    expectedStatus: 201,
    headers: authHeader,
  });
  const payoutID = payout.id as string;

  await poll(
    "payout completion",
    async () =>
      apiJson(request, "GET", `/v1/payouts/${payoutID}`, {
        headers: authHeader,
      }) as Promise<JsonValue>,
    (current) => current.status === "completed",
    45_000,
  );

  const dispute = await apiJson(request, "POST", "/v1/disputes", {
    expectedStatus: 201,
    headers: authHeader,
    data: {
      payment_id: secondPaymentID,
      settlement_id: settlementID,
      reason: "fraudulent",
      amount: 15000,
      currency: "INR",
    },
  });
  const disputeID = dispute.id as string;

  await apiJson(request, "POST", `/v1/disputes/${disputeID}/submit-evidence`, {
    expectedStatus: 200,
    headers: authHeader,
    data: {
      proof_url: "https://example.com/evidence/playwright",
      ticket: `pw-${suffix}`,
    },
  });
  await apiJson(request, "POST", `/v1/disputes/${disputeID}/review`, {
    expectedStatus: 200,
    headers: authHeader,
  });
  await apiJson(request, "POST", `/v1/disputes/${disputeID}/resolve`, {
    expectedStatus: 200,
    headers: authHeader,
    data: {
      outcome: "won",
      notes: "playwright resolution",
    },
  });

  const seed: SeedData = {
    merchantID,
    adminKeyID,
    adminKeySecret,
    email,
    password,
    webhookID,
    webhookToken,
    orderID,
    paymentID,
    refundID,
    secondOrderID,
    secondPaymentID,
    settlementID,
    payoutID,
    disputeID,
  };
  await writeSeed(seed);
  return seed;
}
