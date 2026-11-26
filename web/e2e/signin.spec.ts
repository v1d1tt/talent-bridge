import { expect, test, type Page } from "@playwright/test";

// Sign-in end to end: the real server, the built page, and the fake Google
// (cmd/fake-google) instead of accounts.google.com. The server does the real
// flow against it: PKCE, state, nonce, a signed ID token checked with JWKS.

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

async function signInWithFakeGoogle(page: Page, who: { sub: string; email: string; name: string }) {
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/signin$/);
  await page.getByRole("link", { name: "Sign in with Google" }).click();

  // The fake provider's page stands in for Google's account chooser.
  await expect(page.getByRole("heading", { name: "Fake Google sign-in" })).toBeVisible();
  await page.getByLabel("Subject (Google user id)").fill(who.sub);
  await page.getByLabel("Email", { exact: true }).fill(who.email);
  await page.getByLabel("Name", { exact: true }).fill(who.name);
  await page.getByRole("button", { name: "Continue" }).click();
}

test("a new athlete signs in, chooses a role, stays signed in, and signs out", async ({ page, context }) => {
  const sub = `e2e-${Date.now()}`;
  await signInWithFakeGoogle(page, { sub, email: `${sub}@example.com`, name: "Asha Rao" });

  await expect(page).toHaveURL(/\/welcome$/);
  await expect(page.getByText("Welcome, Asha Rao. Choose how you use Talent-Bridge.")).toBeVisible();
  await page.getByRole("radio", { name: /Athlete/ }).check();
  await page.getByRole("button", { name: "Continue" }).click();

  await expect(page).toHaveURL(/\/$/);
  const nav = page.getByRole("navigation", { name: "Main" });
  await expect(nav).toContainText("Asha Rao");
  await expect(nav).toContainText("Athlete");

  // The session cookie is HttpOnly, Secure, SameSite=Lax, with the __Host- prefix.
  const cookie = (await context.cookies()).find((c) => c.name === "__Host-tb_session");
  expect(cookie).toMatchObject({ httpOnly: true, secure: true, sameSite: "Lax", path: "/" });

  await page.reload();
  await expect(nav).toContainText("Asha Rao");

  await nav.getByRole("button", { name: "Sign out" }).click();
  await expect(nav.getByRole("link", { name: "Sign in", exact: true })).toBeVisible();
  expect((await context.cookies()).find((c) => c.name === "__Host-tb_session")).toBeUndefined();
});

test("the admin email gets the admin role without a choice", async ({ page }) => {
  await signInWithFakeGoogle(page, { sub: `e2e-admin-${Date.now()}`, email: "admin@example.com", name: "Owner" });
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("navigation", { name: "Main" })).toContainText("Admin");
});
