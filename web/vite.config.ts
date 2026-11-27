import { readFileSync } from "node:fs";
import solid from "@solidjs/vite-plugin";
import { defineConfig } from "vitest/config";

// The MediaPipe files are served from /mediapipe/<version>/ (scripts/mediapipe.mjs).
const mediapipe = JSON.parse(readFileSync("node_modules/@mediapipe/tasks-vision/package.json", "utf8")) as { version: string };

export default defineConfig({
  plugins: [solid()],
  define: { __MEDIAPIPE_VERSION__: JSON.stringify(mediapipe.version) },
  server: {
    // `npm run dev` serves the page on :5173 and sends /api and /auth to the Go server on :8080.
    proxy: { "/api": "http://localhost:8080", "/auth": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    globals: false,
    setupFiles: ["./vitest-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
