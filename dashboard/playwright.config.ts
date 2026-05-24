import path from "node:path";

import { defineConfig, devices } from "@playwright/test";

const rootDir = path.resolve(__dirname, "..");
const apiBaseUrl = process.env.API_BASE_URL || "http://127.0.0.1:38090";
const appBaseUrl = process.env.APP_BASE_URL || "http://127.0.0.1:33001";
const databaseUrl = process.env.DATABASE_URL || "postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable";
const redisAddr = process.env.REDIS_ADDR || "localhost:6380";
const kafkaBrokers = process.env.KAFKA_BROKERS || "localhost:19092";
const authDir = path.join(__dirname, "playwright", ".auth");

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 90_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : [["list"], ["html", { open: "never" }]],
  outputDir: "./test-results",
  globalSetup: "./tests/e2e/global-setup.ts",
  use: {
    baseURL: appBaseUrl,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "setup",
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        storageState: path.join(authDir, "dashboard-user.json"),
      },
      dependencies: ["setup"],
    },
  ],
  webServer: [
    {
      command: [
        "env",
        "-u",
        "NO_COLOR",
        "PORT=38090",
        `DATABASE_URL=${databaseUrl}`,
        `REDIS_ADDR=${redisAddr}`,
        `KAFKA_BROKERS=${kafkaBrokers}`,
        "DASHBOARD_ORIGIN=http://127.0.0.1:33001",
        "go",
        "run",
        "./cmd/api-gateway",
      ].join(" "),
      cwd: rootDir,
      url: `${apiBaseUrl}/readyz`,
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: `env -u NO_COLOR sh -c 'pnpm build && API_BASE_URL=${apiBaseUrl} APP_BASE_URL=${appBaseUrl} pnpm start -p 33001'`,
      cwd: __dirname,
      url: `${appBaseUrl}/`,
      reuseExistingServer: false,
      timeout: 180_000,
    },
  ],
});
