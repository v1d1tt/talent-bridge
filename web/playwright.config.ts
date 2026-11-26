import { defineConfig, devices } from "@playwright/test";

// The smoke test runs against the real Go server serving the built page.
// CI builds both first; locally run `make e2e` from the repo root.
const port = Number(process.env.E2E_PORT ?? 8090);
const googlePort = Number(process.env.E2E_GOOGLE_PORT ?? 9998);

export default defineConfig({
  testDir: "./e2e",
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  // Two processes: the fake Google sign-in (cmd/fake-google), then the app,
  // which trusts the fake only because both run on localhost.
  webServer: [
    {
      command: process.env.E2E_FAKE_GOOGLE_CMD ?? "../bin/fake-google",
      url: `http://localhost:${googlePort}/.well-known/openid-configuration`,
      reuseExistingServer: !process.env.CI,
      env: { FAKE_GOOGLE_PORT: String(googlePort) },
      stdout: "pipe",
    },
    {
      command: process.env.E2E_SERVER_CMD ?? "../bin/server",
      url: `http://localhost:${port}/api/health`,
      reuseExistingServer: !process.env.CI,
      env: {
        PORT: String(port),
        STATIC_DIR: "dist",
        DATABASE_URL: process.env.TEST_DATABASE_URL ?? "",
        PUBLIC_URL: `http://localhost:${port}`,
        GOOGLE_ISSUER: `http://localhost:${googlePort}`,
        GOOGLE_CLIENT_ID: "fake-client",
        GOOGLE_CLIENT_SECRET: "fake-secret",
        ADMIN_EMAILS: "admin@example.com",
      },
      stdout: "pipe",
    },
  ],
});
