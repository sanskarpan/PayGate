import { expect, test } from "@playwright/test";

test.use({ storageState: { cookies: [], origins: [] } });

test("public landing renders the premium hero and operator sign-in without browser errors", async ({
  page,
}) => {
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

  await page.goto("/", { waitUntil: "domcontentloaded" });

  await expect(
    page.getByRole("heading", { name: "Move money with a premium operating layer." }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Enter PayGate" })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Authenticate into the merchant operating picture" }),
  ).toBeVisible();

  expect(consoleErrors).toEqual([]);
  expect(pageErrors).toEqual([]);
});
