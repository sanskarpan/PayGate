import { expect, test as setup } from "@playwright/test";

import { ensureSeedData, storageStatePath } from "./helpers/seed";

setup("seed merchant data and authenticate dashboard session", async ({ page, request }) => {
  const seed = await ensureSeedData(request);

  await page.goto("/");
  await page.getByLabel("Merchant ID").fill(seed.merchantID);
  await page.getByLabel("User Email").fill(seed.email);
  await page.getByLabel("Password").fill(seed.password);
  await page.getByRole("button", { name: "Open dashboard" }).click();

  await page.waitForURL("**/overview");
  await expect(page.getByRole("heading", { name: "Merchant command center." })).toBeVisible();

  await page.context().storageState({ path: storageStatePath });
});
