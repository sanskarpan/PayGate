import fs from "node:fs/promises";
import path from "node:path";
import { execFileSync } from "node:child_process";

async function ensureDir(dir: string) {
  await fs.mkdir(dir, { recursive: true });
}

async function main() {
  const dashboardDir = path.resolve(__dirname, "../..");
  const repoRoot = path.resolve(dashboardDir, "..");
  const authDir = path.join(dashboardDir, "playwright", ".auth");

  await ensureDir(authDir);
  await fs.rm(path.join(dashboardDir, "playwright-report"), { recursive: true, force: true });
  await fs.rm(path.join(dashboardDir, "test-results"), { recursive: true, force: true });

  if (process.env.PLAYWRIGHT_SKIP_BOOTSTRAP === "true") {
    return;
  }

  execFileSync("bash", ["scripts/test/prepare_local_stack.sh"], {
    cwd: repoRoot,
    stdio: "inherit",
    env: {
      ...process.env,
      DATABASE_URL: process.env.DATABASE_URL || "postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable",
    },
  });
}

export default main;
