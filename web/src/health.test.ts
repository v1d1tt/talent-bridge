import { afterEach, describe, expect, test, vi } from "vitest";
import { fetchHealth, ServerUnreachableError, type HealthReport } from "./health";

const up: HealthReport = {
  status: "ok",
  version: "abc1234",
  database: { status: "up", latency_ms: 12, schema_version: 1 },
};
const down: HealthReport = {
  status: "degraded",
  version: "abc1234",
  database: { status: "down", latency_ms: 10000, schema_version: 0 },
};

function respond(status: number, body: unknown) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(body), { status })));
}

afterEach(() => vi.unstubAllGlobals());

describe("fetchHealth", () => {
  test("returns the report when the database is up", async () => {
    respond(200, up);
    await expect(fetchHealth()).resolves.toEqual(up);
  });

  test("returns the report on 503, because the server itself answered", async () => {
    respond(503, down);
    await expect(fetchHealth()).resolves.toEqual(down);
  });

  test("throws when the network fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Promise.reject(new TypeError("Failed to fetch"))));
    await expect(fetchHealth()).rejects.toBeInstanceOf(ServerUnreachableError);
  });

  test("throws on a gateway error page", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("<html>Bad gateway</html>", { status: 502 })));
    await expect(fetchHealth()).rejects.toBeInstanceOf(ServerUnreachableError);
  });

  test("throws when the body is not a health report", async () => {
    respond(200, { hello: "world" });
    await expect(fetchHealth()).rejects.toBeInstanceOf(ServerUnreachableError);
  });
});
