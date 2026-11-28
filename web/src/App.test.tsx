import { cleanup, fireEvent, render } from "@solidjs/testing-library";
import { afterEach, describe, expect, test, vi } from "vitest";
import { memoryHistory } from "@solidjs/router";
import { App, makeRouter } from "./App";
import type { HealthReport } from "./health";
import type { Me } from "./me";

const up: HealthReport = { status: "ok", version: "abc1234", database: { status: "up", latency_ms: 42, schema_version: 1 } };
const down: HealthReport = { status: "degraded", version: "abc1234", database: { status: "down", latency_ms: 10000, schema_version: 0 } };

type Reply = () => Promise<Response>;

/**
 * Stubs fetch by path. Each path gets a list of replies, used in order; the
 * last one repeats. /api/me answers {"user": null} (signed out) unless a test sets it.
 */
function serve(routes: Record<string, Reply | Reply[]> = {}) {
  const queues = new Map(Object.entries(routes).map(([k, v]) => [k, Array.isArray(v) ? [...v] : [v]]));
  if (!queues.has("/api/me")) queues.set("/api/me", [json(200, { user: null })]);
  const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
    const path = String(input);
    const q = queues.get(path);
    if (!q) return Promise.reject(new Error(`unexpected fetch ${path}`));
    return (q.length > 1 ? q.shift()! : q[0])();
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
const json = (status: number, body: unknown) => () => Promise.resolve(new Response(JSON.stringify(body), { status }));
const noContent = () => Promise.resolve(new Response(null, { status: 204 }));
const networkError = () => Promise.reject(new TypeError("Failed to fetch"));
const callsTo = (fetchMock: ReturnType<typeof serve>, path: string) =>
  fetchMock.mock.calls.filter(([input]) => String(input) === path);

const me = (role: Me["user"]["role"]): Me => ({
  user: { id: "u1", email: "asha@example.com", display_name: "Asha Rao", role },
  csrf_token: "csrf-123",
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("home page", () => {
  test("says what the app does, links to Settings, and calls only /api/me", async () => {
    const fetchMock = serve();
    const { findByRole, getByRole, queryByText } = render(() => <App router={makeRouter(memoryHistory("/"))} />);
    expect(await findByRole("heading", { level: 1, name: /scored against the pros/ })).toBeInTheDocument();
    expect(getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/settings");
    expect(queryByText("System status")).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual(["/api/me"]);
  });

  test("an unknown path shows a not-found page", async () => {
    serve();
    const { findByRole } = render(() => <App router={makeRouter(memoryHistory("/nope"))} />);
    expect(await findByRole("heading", { level: 1, name: "Page not found" })).toBeInTheDocument();
  });
});

describe("settings page: system status", () => {
  test("shows a connected database with latency and schema version", async () => {
    serve({ "/api/health": json(200, up) });
    const { findByText, getByText } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText("Connected")).toBeInTheDocument();
    expect(getByText("42 ms")).toBeInTheDocument();
    expect(getByText("Migration 1")).toBeInTheDocument();
    expect(getByText("abc1234")).toBeInTheDocument();
    expect(getByText("All parts are connected.")).toBeInTheDocument();
  });

  test("shows a fault mark when the database is down, and the server as answering", async () => {
    serve({ "/api/health": json(503, down) });
    const { findByText, getByText } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText(/cannot reach the database/)).toBeInTheDocument();
    expect(getByText("Answering")).toBeInTheDocument();
    expect(getByText("Not measured")).toBeInTheDocument();
  });

  test("shows the server as not reachable, and Check again retries", async () => {
    const fetchMock = serve({ "/api/health": [networkError, json(200, up)] });
    const { findByText, findByRole } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText(/did not answer/)).toBeInTheDocument();

    fireEvent.click(await findByRole("button", { name: "Check again" }));
    expect(await findByText("Connected")).toBeInTheDocument();
    expect(callsTo(fetchMock, "/api/health")).toHaveLength(2);
  });

  test("Check again refreshes a good report", async () => {
    const fetchMock = serve({ "/api/health": [json(200, up), json(503, down)] });
    const { findByText, findByRole } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText("Connected")).toBeInTheDocument();

    fireEvent.click(await findByRole("button", { name: "Check again" }));
    expect(await findByText(/cannot reach the database/)).toBeInTheDocument();
    expect(callsTo(fetchMock, "/api/health")).toHaveLength(2);
  });
});

describe("header", () => {
  test("signed out: shows Settings and Sign in", async () => {
    serve();
    const { findByRole } = render(() => <App router={makeRouter(memoryHistory("/"))} />);
    expect(await findByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/signin");
    expect(await findByRole("link", { name: "Settings" })).toBeInTheDocument();
  });

  test("signed in: shows the name and role, and Sign out sends the CSRF token", async () => {
    const fetchMock = serve({
      "/api/me": [json(200, me("coach")), json(200, { user: null })],
      "/auth/signout": noContent,
    });
    const { findByText, findByRole, getByText } = render(() => <App router={makeRouter(memoryHistory("/"))} />);
    expect(await findByText("Asha Rao")).toBeInTheDocument();
    expect(getByText("Coach")).toBeInTheDocument();

    fireEvent.click(await findByRole("button", { name: "Sign out" }));
    expect(await findByRole("link", { name: "Sign in" })).toBeInTheDocument();
    const [call] = callsTo(fetchMock, "/auth/signout");
    expect(call?.[1]?.method).toBe("POST");
    expect(new Headers(call?.[1]?.headers).get("X-CSRF-Token")).toBe("csrf-123");
  });
});

describe("sign-in page", () => {
  test("links to the server's Google start, outside the router", async () => {
    serve();
    const { findByRole } = render(() => <App router={makeRouter(memoryHistory("/signin"))} />);
    const google = await findByRole("link", { name: "Sign in with Google" });
    expect(google).toHaveAttribute("href", "/auth/google/start");
    expect(google).toHaveAttribute("target", "_self");
  });

  test.each([
    ["banned", "This account cannot sign in. The admin blocked it."],
    ["cancelled", "Sign-in was cancelled. Try again when you are ready."],
    ["something-new", "Google sign-in did not complete. Try again."],
  ])("explains error=%s", async (code, text) => {
    serve();
    const { findByRole } = render(() => <App router={makeRouter(memoryHistory(`/signin?error=${code}`))} />);
    expect(await findByRole("alert")).toHaveTextContent(text);
  });
});

describe("welcome page: role choice", () => {
  test("needs a choice, then saves it with the CSRF token and goes home", async () => {
    const fetchMock = serve({
      "/api/me": [json(200, me(null)), json(200, me("athlete"))],
      "/api/me/role": json(200, me("athlete")),
    });
    const { findByRole, findByText, getByRole } = render(() => <App router={makeRouter(memoryHistory("/welcome"))} />);
    expect(await findByText("Welcome, Asha Rao. Choose how you use Talent-Bridge.")).toBeInTheDocument();

    fireEvent.click(getByRole("button", { name: "Continue" }));
    expect(await findByText("Choose athlete or coach first.")).toBeInTheDocument();
    expect(callsTo(fetchMock, "/api/me/role")).toHaveLength(0);

    fireEvent.click(getByRole("radio", { name: /Athlete/ }));
    fireEvent.click(getByRole("button", { name: "Continue" }));
    expect(await findByRole("heading", { level: 1, name: /scored against the pros/ })).toBeInTheDocument();
    const [call] = callsTo(fetchMock, "/api/me/role");
    expect(call?.[1]?.body).toBe(JSON.stringify({ role: "athlete" }));
    expect(new Headers(call?.[1]?.headers).get("X-CSRF-Token")).toBe("csrf-123");
  });

  test("a user with a role sees it instead of the form", async () => {
    serve({ "/api/me": json(200, me("coach")) });
    const { findByText, queryByRole } = render(() => <App router={makeRouter(memoryHistory("/welcome"))} />);
    expect(await findByText("Your role is Coach.")).toBeInTheDocument();
    expect(queryByRole("radio")).not.toBeInTheDocument();
  });

  test("signed out: asks to sign in first", async () => {
    serve();
    const { findByText } = render(() => <App router={makeRouter(memoryHistory("/welcome"))} />);
    expect(await findByText("Sign in first, then choose your role.")).toBeInTheDocument();
  });
});

describe("reference register (admin)", () => {
  const gate = (shot: string, name: string, discipline: "batting" | "bowling", view: "side_on" | "front_on", references: number, legacy = 0) => ({
    shot, shot_name: name, discipline, view, references, legacy_references: legacy, lopo_passed: false, enabled: false,
  });
  const gates = {
    min_references: 3,
    gates: [
      gate("fast_bowling", "Fast bowling", "bowling", "side_on", 3),
      gate("fast_bowling", "Fast bowling", "bowling", "front_on", 0, 6),
      gate("pull", "Pull", "batting", "side_on", 1),
      gate("pull", "Pull", "batting", "front_on", 0),
    ],
  };
  const ref = (id: string, player: string, source: string, origin: "browser" | "legacy_csv") => ({
    id, shot: "pull", shot_name: "Pull", discipline: "batting", view: "side_on", player, handedness: "right",
    source, origin, fps: 30, frame_count: 60, duration_s: 2, created_at: "2026-11-28T06:00:00Z",
  });

  test("a coach is told the page is admin only, and nothing admin is fetched", async () => {
    const fetchMock = serve({ "/api/me": json(200, me("coach")) });
    const { findByText, queryByRole } = render(() => <App router={makeRouter(memoryHistory("/admin/references"))} />);
    expect(await findByText("Only the admin can see the reference register.")).toBeInTheDocument();
    expect(queryByRole("link", { name: "References" })).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual(["/api/me"]);
  });

  test("the admin sees the gates with a needs-3+ mark, old CSVs apart, and the register", async () => {
    serve({
      "/api/me": json(200, me("admin")),
      "/api/admin/shot-gates": json(200, gates),
      "/api/admin/references": json(200, {
        references: [
          ref("r1", "Rohit Sharma", "2026 hackathon CSV Rohit_Pull_Shot.csv; source video unknown", "legacy_csv"),
          ref("r2", "Test Pro", "https://example.com/clip", "browser"),
          ref("r3", "Sneaky", "javascript:alert(1)", "browser"),
        ],
      }),
    });
    const { findByRole, getByRole, getByText } = render(() => <App router={makeRouter(memoryHistory("/admin/references"))} />);
    expect(await findByRole("heading", { level: 1, name: "Reference register" })).toBeInTheDocument();
    expect(await findByRole("link", { name: "References" })).toHaveAttribute("href", "/admin/references");

    const fast = await findByRole("row", { name: /Fast bowling/ });
    expect(fast).toHaveTextContent("3 references");
    expect(fast).toHaveTextContent("ready for the test");
    expect(fast).toHaveTextContent("0 references");
    expect(fast).toHaveTextContent("needs 3+");
    expect(fast).toHaveTextContent("+6 old CSV, not counted");
    expect(getByRole("row", { name: /^Pull/ })).toHaveTextContent("1 reference");

    // The register: an http(s) source is a link; any other text is not.
    expect(await findByRole("row", { name: /Rohit Sharma/ })).toHaveTextContent("old CSV");
    expect(getByRole("link", { name: "https://example.com/clip" })).toHaveAttribute("rel", "noopener noreferrer nofollow");
    expect(getByText("javascript:alert(1)").closest("a")).toBeNull();
    expect(getByRole("link", { name: "Stick figure: Test Pro, Pull" })).toHaveAttribute("href", "/admin/references/r2");

    // The form offers the shots from the gates, grouped by discipline.
    const select = getByRole("combobox", { name: /Shot/ }) as HTMLSelectElement;
    expect([...select.options].map((o) => o.value)).toEqual(["", "fast_bowling", "pull"]);
  });

  test("saving needs a read clip first", async () => {
    const fetchMock = serve({
      "/api/me": json(200, me("admin")),
      "/api/admin/shot-gates": json(200, gates),
      "/api/admin/references": json(200, { references: [] }),
    });
    const { findByRole, getByRole } = render(() => <App router={makeRouter(memoryHistory("/admin/references"))} />);
    const form = (await findByRole("button", { name: "Save reference" })).closest("form")!;
    fireEvent.submit(form);
    expect(await findByRole("alert")).toHaveTextContent("Choose a pro clip first.");
    expect(getByRole("button", { name: "Save reference" })).toBeEnabled();
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toEqual([]);
  });
});
