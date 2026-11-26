// The browser side of GET /api/health.

export type HealthReport = {
  status: "ok" | "degraded";
  version: string;
  database: {
    status: "up" | "down";
    latency_ms: number;
    schema_version: number;
  };
};

/** The Go server did not answer with a health report (it is down, or a proxy answered). */
export class ServerUnreachableError extends Error {
  constructor(detail: string) {
    super(`server not reachable: ${detail}`);
    this.name = "ServerUnreachableError";
  }
}

/**
 * Fetches the health report. A 503 still carries a report (the server is up,
 * the database is not), so only a missing or malformed report throws.
 */
export async function fetchHealth(): Promise<HealthReport> {
  let res: Response;
  try {
    res = await fetch("/api/health", { headers: { Accept: "application/json" }, cache: "no-store" });
  } catch (err) {
    throw new ServerUnreachableError(String(err));
  }
  if (res.status !== 200 && res.status !== 503) {
    throw new ServerUnreachableError(`HTTP ${res.status}`);
  }
  const body: unknown = await res.json().catch(() => null);
  if (!isHealthReport(body)) {
    throw new ServerUnreachableError("unexpected response body");
  }
  return body;
}

function isHealthReport(v: unknown): v is HealthReport {
  if (typeof v !== "object" || v === null) return false;
  const r = v as Record<string, unknown>;
  const db = r.database as Record<string, unknown> | undefined;
  return (
    (r.status === "ok" || r.status === "degraded") &&
    typeof r.version === "string" &&
    typeof db === "object" &&
    db !== null &&
    (db.status === "up" || db.status === "down") &&
    typeof db.latency_ms === "number" &&
    typeof db.schema_version === "number"
  );
}
