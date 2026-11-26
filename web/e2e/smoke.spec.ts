import { expect, test } from "@playwright/test";

// Every test fails on a script error or a CSP violation in the browser console.
const errors = new WeakMap<object, string[]>();

test.beforeEach(async ({ page }) => {
  const list: string[] = [];
  errors.set(page, list);
  page.on("console", (msg) => msg.type() === "error" && list.push(msg.text()));
  page.on("pageerror", (err) => list.push(err.message));
});

test.afterEach(async ({ page }) => {
  expect(errors.get(page)).toEqual([]);
});

test("home page says what the app does and links to Settings", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveTitle("Talent-Bridge");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Your technique, scored against the pros");
  await expect(page.getByRole("heading", { name: "System status" })).toHaveCount(0);
});

test("Settings shows a connected database, end to end", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: "Settings" }).click();

  await expect(page).toHaveURL(/\/settings$/);
  await expect(page.getByRole("heading", { level: 2, name: "System status" })).toBeVisible();
  await expect(page.getByRole("row", { name: /Database/ })).toContainText("Connected");
  await expect(page.getByRole("row", { name: /Schema/ })).toContainText("Migration 1");
  await expect(page.getByRole("status")).toHaveText("All parts are connected.");
});

test("a deep link to /settings loads directly (server falls back to index.html)", async ({ page }) => {
  await page.goto("/settings");
  await expect(page.getByRole("row", { name: /Database/ })).toContainText("Connected");
});
