// The browser side of the auth session: GET /api/me, role choice, sign-out.
// GET /api/me always answers 200: the user, or {"user": null} when signed out.
// The session cookie is HttpOnly, so the page never sees it; it learns who is
// signed in only from /api/me, which also hands out the CSRF token.

export type Role = "athlete" | "coach" | "admin";

export type Me = {
  user: { id: string; email: string; display_name: string; role: Role | null };
  csrf_token: string;
};

/** Who is using the page. `offline` means the server did not answer. */
export type SessionState = { kind: "signed-in"; me: Me } | { kind: "signed-out"; offline?: boolean };

const CSRF_HEADER = "X-CSRF-Token";

export async function fetchMe(): Promise<SessionState> {
  let res: Response;
  try {
    res = await fetch("/api/me", { headers: { Accept: "application/json" }, cache: "no-store" });
  } catch {
    return { kind: "signed-out", offline: true };
  }
  if (res.status !== 200) {
    return { kind: "signed-out", offline: true };
  }
  const body: unknown = await res.json().catch(() => null);
  if (isMe(body)) return { kind: "signed-in", me: body };
  // Signed out (or banned: the server already deleted the session) is {"user": null}.
  const signedOut = typeof body === "object" && body !== null && (body as { user?: unknown }).user === null;
  return { kind: "signed-out", offline: !signedOut };
}

/** Thrown when the server refuses a change; `code` is the server's error code. */
export class RequestError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`request failed: ${status} ${code}`);
    this.name = "RequestError";
  }
}

async function post(path: string, csrf: string, body?: unknown): Promise<Response> {
  const res = await fetch(path, {
    method: "POST",
    headers: { [CSRF_HEADER]: csrf, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const err: unknown = await res.json().catch(() => null);
    const code = typeof err === "object" && err && "error" in err ? String(err.error) : "unknown";
    throw new RequestError(res.status, code);
  }
  return res;
}

/** The one-time choice at first sign-in. */
export async function chooseRole(role: "athlete" | "coach", csrf: string): Promise<void> {
  await post("/api/me/role", csrf, { role });
}

/** Deletes the auth session on the server and clears the cookie. */
export async function signOut(csrf: string): Promise<void> {
  await post("/auth/signout", csrf);
}

function isMe(v: unknown): v is Me {
  if (typeof v !== "object" || v === null) return false;
  const m = v as Record<string, unknown>;
  const u = m.user as Record<string, unknown> | undefined;
  return (
    typeof m.csrf_token === "string" &&
    typeof u === "object" &&
    u !== null &&
    typeof u.id === "string" &&
    typeof u.email === "string" &&
    typeof u.display_name === "string" &&
    (u.role === null || u.role === "athlete" || u.role === "coach" || u.role === "admin")
  );
}
