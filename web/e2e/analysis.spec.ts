import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, test, type Page, type Request } from "@playwright/test";

// The privacy check of ADR 0001: a real clip goes through the real page,
// the real pose model (MediaPipe, in this Chromium), the real server and
// Azurite. Every request the page makes is recorded, and none may carry the
// video: not in a body, not in a URL.
//
// The fixture is a 2 s, 360p, silent WebM cut of the owner's own side-on
// clip (committed with the owner's consent). WebM, because Playwright's
// Chromium has no H.264 decoder.

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
  expect(errors.get(page)).toEqual([]);
});

async function signInAsAthlete(page: Page) {
  const sub = `e2e-athlete-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  await page.goto("/signin");
  await page.getByRole("link", { name: "Sign in with Google" }).click();
  await page.getByLabel("Subject (Google user id)").fill(sub);
  await page.getByLabel("Email", { exact: true }).fill(`${sub}@example.com`);
  await page.getByLabel("Name", { exact: true }).fill("Clip Tester");
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("radio", { name: /Athlete/ }).check();
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("navigation", { name: "Main" })).toContainText("Athlete");
}

/** Does `haystack` contain any 64-byte slice of the clip (taken at 16 places)? */
function containsClipBytes(haystack: Buffer): boolean {
  for (let k = 1; k <= 16; k++) {
    const at = Math.floor((k * clip.length) / 18);
    if (haystack.includes(clip.subarray(at, at + 64))) return true;
  }
  return false;
}

const VIDEO_MAGIC = [
  Buffer.from([0x1a, 0x45, 0xdf, 0xa3]), // WebM / Matroska (EBML)
  Buffer.from("ftyp"), // MP4 / MOV
];

test("a clip becomes a saved analysis with key frames, and the video never leaves the browser", async ({ page }) => {
  test.setTimeout(180_000); // the pose model runs on the CPU in CI
  await signInAsAthlete(page);

  // Every request, as the page makes it. Chromium does not report some
  // bodies here (a cross-origin binary PUT comes back empty), so the bodies
  // are also read by a route, which holds each network request before it goes.
  const sent: { url: string; method: string; type: string; body: Buffer; req: Request }[] = [];
  page.on("request", (r: Request) => {
    sent.push({ url: r.url(), method: r.method(), type: r.headers()["content-type"] ?? "", body: r.postDataBuffer() ?? Buffer.alloc(0), req: r });
  });
  const routed = new Map<Request, Buffer>();
  await page.route("**/*", async (route) => {
    routed.set(route.request(), route.request().postDataBuffer() ?? Buffer.alloc(0));
    await route.fallback();
  });

  await page.getByRole("link", { name: "New analysis", exact: true }).click();
  await expect(page.getByRole("heading", { name: "New analysis" })).toBeVisible();
  await page.getByLabel("Choose a clip").setInputFiles(clipPath);

  await expect(page.getByRole("status")).toContainText(/Reading frame \d+ of 60/, { timeout: 60_000 });
  await expect(page).toHaveURL(/\/analyses\/[0-9a-f-]{36}$/, { timeout: 150_000 });

  // The result page: the clip's numbers, the plate, and the key frames from signed URLs.
  const sheet = page.getByRole("table");
  await expect(sheet).toContainText("60");
  await expect(sheet).toContainText("30 per second");
  await expect(sheet).toContainText("2.00 s");
  await expect(page.getByText("Scoring comes in a later step.")).toBeVisible();
  const stills = page.getByRole("img", { name: /Still picture of frame/ });
  await expect(stills).toHaveCount(3);
  for (const img of await stills.all()) {
    await expect(img).toHaveJSProperty("complete", true);
    expect(await img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);
    expect(await img.getAttribute("src")).toMatch(/^http:\/\/127\.0\.0\.1:10000\/devstoreaccount1\/key-frames\/analyses(\/|%2F).+\.jpg\?.*sig=/);
  }

  // The privacy check, over every request of the whole flow.
  // blob: and data: reads stay inside the browser (the <video> plays the
  // picked file from a blob: URL). They may only read, never send.
  const local = sent.filter((r) => /^(blob|data):/.test(r.url));
  for (const r of local) {
    expect(r.method, `${r.url.slice(0, 80)} is a local read`).toBe("GET");
    expect(r.body.length).toBe(0);
  }
  const network = sent.filter((r) => !local.includes(r));
  expect(network.length).toBeGreaterThan(0);
  for (const r of network) {
    expect(r.url, "a request goes somewhere other than http(s)").toMatch(/^https?:/);
    // The test must see every byte that was sent: a body it cannot read
    // would pass the checks below unread. The browser's own Content-Length
    // says how many bytes went out.
    const seen = routed.get(r.req);
    expect(seen, `${r.method} ${r.url.slice(0, 80)} was not seen by the route`).toBeDefined();
    if (seen && seen.length > r.body.length) r.body = seen;
    const length = (await r.req.allHeaders())["content-length"];
    if (length !== undefined) {
      expect(r.body.length, `${r.method} ${r.url.slice(0, 80)} sent a body the test cannot read`).toBe(Number(length));
    }
    expect(r.url.length, `${r.url.slice(0, 80)} is too long to be anything but data`).toBeLessThan(2048);
    expect(containsClipBytes(Buffer.from(decodeURIComponent(r.url))), `${r.method} ${r.url} has clip bytes in its URL`).toBe(false);
    expect(containsClipBytes(r.body), `${r.method} ${r.url} sent clip bytes`).toBe(false);
    for (const magic of VIDEO_MAGIC) expect(r.body.includes(magic), `${r.method} ${r.url} sent a video header`).toBe(false);
    expect(r.type).not.toMatch(/^video\/|multipart/);
  }
  // Exactly one JSON save, and the only binary bodies are 3 JPEG PUTs to Blob Storage.
  const saves = network.filter((r) => r.method === "POST" && new URL(r.url).pathname === "/api/analyses");
  expect(saves).toHaveLength(1);
  expect(saves[0].type).toBe("application/json");
  const puts = network.filter((r) => r.method === "PUT");
  expect(puts).toHaveLength(3);
  for (const p of puts) {
    expect(new URL(p.url).host).toBe("127.0.0.1:10000");
    expect(p.type).toBe("image/jpeg");
    expect([...p.body.subarray(0, 3)]).toEqual([0xff, 0xd8, 0xff]);
    expect(p.body.length).toBeLessThan(512 * 1024);
  }
});

test("a coach cannot open the new analysis page's tools", async ({ page }) => {
  const sub = `e2e-coach-${Date.now()}`;
  await page.goto("/signin");
  await page.getByRole("link", { name: "Sign in with Google" }).click();
  await page.getByLabel("Subject (Google user id)").fill(sub);
  await page.getByLabel("Email", { exact: true }).fill(`${sub}@example.com`);
  await page.getByLabel("Name", { exact: true }).fill("Coach Tester");
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("radio", { name: /Coach/ }).check();
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("navigation", { name: "Main" })).not.toContainText("New analysis");
  await page.goto("/analyses/new");
  await expect(page.getByText("Only athletes analyse clips.")).toBeVisible();
  await expect(page.getByLabel("Choose a clip")).toHaveCount(0);
});
