import { expect, test } from "@playwright/test";

import { loadSeedData } from "./helpers/seed";

const pageChecks = [
  { path: "/overview", heading: "Merchant command center." },
  { path: "/orders", heading: "Orders" },
  { path: "/api-keys", heading: "API Keys" },
  { path: "/webhooks", heading: "Webhooks" },
  { path: "/settlements", heading: "Settlement Reports" },
  { path: "/payouts", heading: "Payouts" },
  { path: "/recon", heading: "Reconciliation Control" },
  { path: "/risk", heading: "Risk Events" },
  { path: "/gateway", heading: "Payment Gateway Control Panel" },
  { path: "/observability", heading: "Observability" },
  { path: "/audit", heading: "Audit Log" },
  { path: "/team", heading: "Team" },
  { path: "/disputes", heading: "Disputes" },
];

test("operator dashboard pages render correctly with seeded data", async ({ page }) => {
  const seed = await loadSeedData();
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });
  page.on("pageerror", (err) => {
    pageErrors.push(err.message);
  });

  for (const check of pageChecks) {
    await page.goto(check.path, { waitUntil: "networkidle" });
    await expect(page.getByRole("heading", { name: check.heading })).toBeVisible();
  }

  await page.goto(`/orders/${seed.orderID}`, { waitUntil: "networkidle" });
  await expect(page.getByText("Order Detail")).toBeVisible();
  await expect(page.getByText(seed.orderID)).toBeVisible();

  await page.goto(`/payments/${seed.paymentID}`, { waitUntil: "networkidle" });
  await expect(page.getByText("Payment Trace")).toBeVisible();
  await expect(page.getByText("State History")).toBeVisible();

  await page.goto(`/refunds?payment_id=${seed.paymentID}`, { waitUntil: "networkidle" });
  await expect(page.getByRole("heading", { name: `Refunds for ${seed.paymentID}` })).toBeVisible();
  await expect(page.getByText(seed.refundID)).toBeVisible();

  await page.goto(`/webhooks/${seed.webhookID}`, { waitUntil: "networkidle" });
  await expect(page.getByText("Webhook Subscription")).toBeVisible();
  await expect(page.getByText("Delivery Log")).toBeVisible();

  await page.goto(`/settlements/${seed.settlementID}`, { waitUntil: "networkidle" });
  await expect(page.getByText("Settlement Batch")).toBeVisible();
  await expect(page.getByText("Payment Items")).toBeVisible();

  await page.goto(`/disputes/${seed.disputeID}`, { waitUntil: "networkidle" });
  await expect(page.getByText("Dispute Detail")).toBeVisible();
  await expect(page.getByText("Case summary")).toBeVisible();
  await expect(page.getByText("Submitted evidence payload")).toBeVisible();

  expect(consoleErrors).toEqual([]);
  expect(pageErrors).toEqual([]);
});
