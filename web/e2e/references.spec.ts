import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, test, type Page } from "@playwright/test";
import { unexpected } from "./console";

// The admin's reference flow (slice 4): a pro clip goes through the same
// browser pose code as an athlete's clip, and only the pose sequence is
// saved. No picture of a pro clip is uploaded or shown. The fixture is the
// slice 3 clip (side-on bowling, filmed by the owner's friend, public with consent).

const clipPath = fileURLToPath(new URL("./fixtures/side-on.webm", import.meta.url));
const clip = readFileSync(clipPath);

const errors = new WeakMap<object, string[]>();

test.beforeEach(async ({ page }) => {
  const list: string[] = [];
  errors.set(page, list);
  page.on("console", (msg) => msg.type() === "error" && list.push(msg.text()));
  page.on("pageerror", (err) => list.push(err.message));
});

test.afterEach(async ({ page }) => {
  expect(unexpected(errors.get(page) ?? [])).toEqual([]);
});

async function signIn(page: Page, email: string, role?: "athlete") {
  const sub = `e2e-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  await page.goto("/signin");
  await page.getByRole("link", { name: "Sign in with Google" }).click();
  await page.getByLabel("Subject (Google user id)").fill(sub);
  await page.getByLabel("Email", { exact: true }).fill(email || `${sub}@example.com`);
  await page.getByLabel("Name", { exact: true }).fill("Register Tester");
  await page.getByRole("button", { name: "Continue" }).click();
  if (role) {
    await page.getByRole("radio", { name: /Athlete/ }).check();
    await page.getByRole("button", { name: "Continue" }).click();
  }
  await expect(page.getByRole("navigation", { name: "Main" })).toContainText(role ? "Athlete" : "Admin");
}

/** The number of references in the Fast bowling, side-on cell of the gate table. */
async function fastBowlingSideOn(page: Page): Promise<number> {
  const cell = page.getByRole("row", { name: /^Fast bowling/ }).getByRole("cell").first();
  const text = (await cell.textContent()) ?? "";
  const n = Number(/(\d+) references?/.exec(text)?.[1]);
  expect(Number.isFinite(n), `gate cell reads "${text}"`).toBe(true);
  return n;
}

test("the admin turns a pro clip into a reference; the gate counts it and only a stick figure is shown", async ({ page }) => {
  test.setTimeout(180_000); // the pose model runs on the CPU in CI
  await signIn(page, "admin@example.com");

  const bodies: Buffer[] = [];
  const hosts = new Set<string>();
  await page.route("**/*", async (route) => {
    const r = route.request();
    hosts.add(new URL(r.url()).host || r.url().split(":")[0]);
    bodies.push(r.postDataBuffer() ?? Buffer.alloc(0));
    await route.fallback();
  });

  await page.getByRole("link", { name: "References", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Reference register" })).toBeVisible();
  const before = await fastBowlingSideOn(page);

  const player = `E2E Pro ${Date.now()}`;
  await page.getByLabel("Player").fill(player);
  await page.getByRole("combobox", { name: "Shot" }).selectOption("fast_bowling");
  await page.getByRole("radio", { name: "Side-on" }).check();
  await page.getByRole("radio", { name: "Right" }).check();
  await page.getByLabel("Source").fill("Slice 3 test clip, filmed by the owner");
  await page.getByLabel("Choose a pro clip").setInputFiles(clipPath);

  await expect(page.getByRole("status")).toContainText(/Read 60 frames/, { timeout: 150_000 });
  // The plate is a stick figure on a canvas: no <video> and no picture of the clip.
  await expect(page.locator("form video")).toHaveCount(0);
  await expect(page.locator("form img")).toHaveCount(0);
  await expect(page.getByRole("img", { name: /Frame \d+ of 60/ })).toBeVisible();

  await page.getByRole("button", { name: "Save reference" }).click();
  await expect(page.getByRole("status")).toContainText(`Saved the reference for ${player}.`);
  await expect.poll(() => fastBowlingSideOn(page)).toBe(before + 1);
  await expect(page.getByRole("row", { name: new RegExp(player) })).toContainText("Side-on");

  await page.getByRole("link", { name: "See its stick figure" }).click();
  await expect(page.getByRole("heading", { level: 1, name: player })).toBeVisible();
  await expect(page.getByRole("img", { name: /Frame \d+ of 60/ })).toBeVisible();
  await expect(page.locator("main img, main video")).toHaveCount(0);

  // Privacy (ADR 0001): only this app was called (no Blob Storage: pro key
  // frames are never stored), and no request carried the clip.
  for (const host of hosts) expect(["localhost:8090", "blob"]).toContain(host);
  for (const body of bodies) {
    for (let k = 1; k <= 16; k++) {
      const at = Math.floor((k * clip.length) / 18);
      expect(body.includes(clip.subarray(at, at + 64)), "a request carried clip bytes").toBe(false);
    }
  }
});

test("an athlete cannot open the register, and the admin API refuses them", async ({ page }) => {
  await signIn(page, "", "athlete");
  await page.goto("/admin/references");
  await expect(page.getByText("Only the admin can see the reference register.")).toBeVisible();
  await expect(page.getByRole("link", { name: "References", exact: true })).toHaveCount(0);
  // The page's own cookies, outside the page (a refused fetch in the page would log a console error).
  expect((await page.request.get("/api/admin/references")).status()).toBe(403);
});
