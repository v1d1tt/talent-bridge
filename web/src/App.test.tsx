import { cleanup, fireEvent, render } from "@solidjs/testing-library";
import { afterEach, describe, expect, test, vi } from "vitest";
import { memoryHistory } from "@solidjs/router";
import { App, makeRouter } from "./App";
import type { HealthReport } from "./health";

const up: HealthReport = { status: "ok", version: "abc1234", database: { status: "up", latency_ms: 42, schema_version: 1 } };
const down: HealthReport = { status: "degraded", version: "abc1234", database: { status: "down", latency_ms: 10000, schema_version: 0 } };

function serve(...responses: Array<() => Promise<Response>>) {
  const fetchMock = vi.fn();
  for (const r of responses) fetchMock.mockImplementationOnce(r);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
const json = (status: number, body: unknown) => () => Promise.resolve(new Response(JSON.stringify(body), { status }));
const networkError = () => Promise.reject(new TypeError("Failed to fetch"));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("home page", () => {
  test("says what the app does, links to Settings, and does not call the API", async () => {
    const fetchMock = serve();
    const { findByRole, getByRole, queryByText } = render(() => <App router={makeRouter(memoryHistory("/"))} />);
    expect(await findByRole("heading", { level: 1, name: /scored against the pros/ })).toBeInTheDocument();
    expect(getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/settings");
    expect(queryByText("System status")).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  test("an unknown path shows a not-found page", async () => {
    serve();
    const { findByRole } = render(() => <App router={makeRouter(memoryHistory("/nope"))} />);
    expect(await findByRole("heading", { level: 1, name: "Page not found" })).toBeInTheDocument();
  });
});

describe("settings page: system status", () => {
  test("shows a connected database with latency and schema version", async () => {
    serve(json(200, up));
    const { findByText, getByText } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText("Connected")).toBeInTheDocument();
    expect(getByText("42 ms")).toBeInTheDocument();
    expect(getByText("Migration 1")).toBeInTheDocument();
    expect(getByText("abc1234")).toBeInTheDocument();
    expect(getByText("All parts are connected.")).toBeInTheDocument();
  });

  test("shows a fault mark when the database is down, and the server as answering", async () => {
    serve(json(503, down));
    const { findByText, getByText } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText(/cannot reach the database/)).toBeInTheDocument();
    expect(getByText("Answering")).toBeInTheDocument();
    expect(getByText("Not measured")).toBeInTheDocument();
  });

  test("shows the server as not reachable, and Check again retries", async () => {
    const fetchMock = serve(networkError, json(200, up));
    const { findByText, findByRole } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText(/did not answer/)).toBeInTheDocument();

    fireEvent.click(await findByRole("button", { name: "Check again" }));
    expect(await findByText("Connected")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  test("Check again refreshes a good report", async () => {
    const fetchMock = serve(json(200, up), json(503, down));
    const { findByText, findByRole } = render(() => <App router={makeRouter(memoryHistory("/settings"))} />);
    expect(await findByText("Connected")).toBeInTheDocument();

    fireEvent.click(await findByRole("button", { name: "Check again" }));
    expect(await findByText(/cannot reach the database/)).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
