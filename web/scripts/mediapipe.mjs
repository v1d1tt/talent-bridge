// Copies the MediaPipe WebAssembly files and the pose models into public/,
// so the app serves them from its own origin (no third-party CDN; the CSP
// stays 'self'). Runs before `vite build` and `vite`.
//
//   public/mediapipe/<tasks-vision version>/wasm/vision_wasm_internal.{js,wasm}
//   public/mediapipe/models/pose_landmarker_{lite,full}_float16_1.task
//   public/mediapipe/LICENSE, NOTICE (Apache 2.0 terms, kept with the files)
//
// Each file also gets a .gz copy; the Go server sends it to browsers that
// accept gzip (the 14 MB .wasm becomes about 4 MB). The models are checked
// against pinned SHA-256 sums, so a changed file at the source fails the build.
import { createHash } from "node:crypto";
import { copyFile, mkdir, readFile, stat, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const pkg = JSON.parse(await readFile(join(web, "node_modules/@mediapipe/tasks-vision/package.json"), "utf8"));
const out = join(web, "public/mediapipe");

const models = {
  lite: "59929e1d1ee95287735ddd833b19cf4ac46d29bc7afddbbf6753c459690d574a",
  full: "5134a3aad27a58b93da0088d431f366da362b44e3ccfbe3462b3827a839011b1",
};

async function exists(p) {
  return stat(p).then(() => true, () => false);
}

async function withGzip(path) {
  await writeFile(path + ".gz", gzipSync(await readFile(path), { level: 9 }));
}

const sha256 = (b) => createHash("sha256").update(b).digest("hex");

// 1. The WebAssembly runtime, from the installed npm package (version pinned in package.json).
const wasmDir = join(out, pkg.version, "wasm");
await mkdir(wasmDir, { recursive: true });
for (const f of ["vision_wasm_internal.js", "vision_wasm_internal.wasm"]) {
  const dest = join(wasmDir, f);
  if (!(await exists(dest + ".gz"))) {
    await copyFile(join(web, "node_modules/@mediapipe/tasks-vision/wasm", f), dest);
    await withGzip(dest);
  }
}

// 2. The models, downloaded once and checked.
await mkdir(join(out, "models"), { recursive: true });
for (const [name, sum] of Object.entries(models)) {
  const dest = join(out, "models", `pose_landmarker_${name}_float16_1.task`);
  if ((await exists(dest)) && sha256(await readFile(dest)) === sum) continue;
  const url = `https://storage.googleapis.com/mediapipe-models/pose_landmarker/pose_landmarker_${name}/float16/1/pose_landmarker_${name}.task`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`download ${url}: ${res.status}`);
  const body = Buffer.from(await res.arrayBuffer());
  if (sha256(body) !== sum) throw new Error(`${url}: SHA-256 is ${sha256(body)}, want ${sum}`);
  await writeFile(dest, body);
  await withGzip(dest);
}

// 3. The licence and notice travel with the files.
for (const f of ["LICENSE", "NOTICE"]) await copyFile(join(web, "third_party/mediapipe", f), join(out, f));

console.log(`mediapipe: tasks-vision ${pkg.version} wasm + lite/full models in public/mediapipe`);
