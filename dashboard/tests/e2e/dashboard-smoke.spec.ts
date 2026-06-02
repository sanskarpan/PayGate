import { expect, test } from "@playwright/test";

import { loadSeedData } from "./helpers/seed";

const pageChecks = [
  { path: "/overview", heading: "Merchant command center." },
  { path: "/orders", heading: "Orders" },
  { path: "/compliance", heading: "Onboarding and controls." },
  { path: "/api-keys", text: "Create Key" },
  { path: "/webhooks", heading: "Webhooks" },
  { path: "/settlements", heading: "Settlement Reports" },
  { path: "/payouts", heading: "Bank payout operations." },
  { path: "/reports", heading: "Download center." },
  { path: "/recon", heading: "Reconciliation Control" },
  { path: "/risk", heading: "Manual pressure queue." },
  { path: "/gateway", heading: "Failure-path command deck." },
  { path: "/observability", heading: "Observability" },
  { path: "/audit", heading: "Audit event trail." },
  { path: "/team", heading: "Team access fabric." },
  { path: "/disputes", heading: "Disputes" },
] as const;

const ignoredConsoleErrorPatterns = [
  "Failed to fetch RSC payload",
  "Falling back to browser navigation",
];

test("operator dashboard pages render correctly with seeded data", async ({ page }) => {
  const seed = await loadSeedData();
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      const text = msg.text();
      if (ignoredConsoleErrorPatterns.some((pattern) => text.includes(pattern))) {
        return;
      }
      consoleErrors.push(text);
    }
  });
  page.on("pageerror", (err) => {
    pageErrors.push(err.message);
  });

  const navigate = async (path: string) => {
    await page.goto(path, { waitUntil: "domcontentloaded" });
  };

  for (const check of pageChecks) {
    await navigate(check.path);
    if ("heading" in check) {
      await expect(page.getByRole("heading", { name: check.heading })).toBeVisible();
      continue;
    }
    await expect(page.getByText(check.text)).toBeVisible();
  }

  await navigate(`/orders/${seed.orderID}`);
  await expect(page.getByText("Order Dossier")).toBeVisible();
  await expect(page.getByText(seed.orderID)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Amount ledger" })).toBeVisible();

  await navigate(`/payments/${seed.paymentID}`);
  await expect(page.getByText("Payment Trace")).toBeVisible();
  await expect(page.getByText("State History")).toBeVisible();

  await navigate(`/refunds?payment_id=${seed.paymentID}`);
  await expect(page.getByRole("heading", { name: `Refunds for ${seed.paymentID}` })).toBeVisible();
  await expect(page.getByText(seed.refundID)).toBeVisible();

  await navigate(`/webhooks/${seed.webhookID}`);
  await expect(page.getByText("Webhook Subscription", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Delivery Log" })).toBeVisible();

  await navigate(`/settlements/${seed.settlementID}`);
  await expect(page.getByText("Settlement Batch", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: /Payment Items/ })).toBeVisible();

  await navigate(`/disputes/${seed.disputeID}`);
  await expect(page.getByText("Dispute Detail", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Case summary" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Submitted evidence payload" })).toBeVisible();

  await navigate("/compliance");
  await expect(page.getByRole("heading", { name: "Onboarding decision" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Merchant live rails" })).toBeVisible();

  await navigate("/risk");
  await expect(page.getByRole("heading", { name: "Workbench controls" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Risk event ledger" })).toBeVisible();

  await navigate("/payouts");
  await expect(page.getByRole("heading", { name: "Treasury workbench" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Payout ledger" })).toBeVisible();

  expect(consoleErrors).toEqual([]);
  expect(pageErrors).toEqual([]);
});
