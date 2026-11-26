import solid from "@solidjs/vite-plugin";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [solid()],
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
