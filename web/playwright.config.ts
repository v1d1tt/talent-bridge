import { defineConfig, devices } from "@playwright/test";

// The smoke test runs against the real Go server serving the built page.
// CI builds both first; locally run `make e2e` from the repo root.
const port = Number(process.env.E2E_PORT ?? 8090);

export default defineConfig({
  testDir: "./e2e",
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: process.env.E2E_SERVER_CMD ?? "../bin/server",
    url: `http://localhost:${port}/api/health`,
    reuseExistingServer: !process.env.CI,
    env: { PORT: String(port), STATIC_DIR: "dist", DATABASE_URL: process.env.TEST_DATABASE_URL ?? "" },
    stdout: "pipe",
  },
});
