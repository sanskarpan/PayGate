import fs from "node:fs/promises";
import path from "node:path";

async function ensureDir(dir: string) {
  await fs.mkdir(dir, { recursive: true });
}

async function main() {
  const dashboardDir = path.resolve(__dirname, "../..");
  const authDir = path.join(dashboardDir, "playwright", ".auth");

  await ensureDir(authDir);
  await fs.rm(path.join(dashboardDir, "playwright-report"), { recursive: true, force: true });
  await fs.rm(path.join(dashboardDir, "test-results"), { recursive: true, force: true });

  if (process.env.PLAYWRIGHT_SKIP_BOOTSTRAP === "true") {
    return;
  }
}

export default main;
