// The browser side of the admin's reference register: save a pro
// reference (the pose sequence only; the clip stays on the device), list
// the register, read one reference, and read the shot gates.
import { AnalysisError } from "./analyses";
import type { PoseSequence } from "./pose/types";

const CSRF_HEADER = "X-CSRF-Token";

export type View = "side_on" | "front_on";
export type Handedness = "right" | "left";
export type Discipline = "batting" | "bowling";

export type Reference = {
  id: string;
  shot: string;
  shot_name: string;
  discipline: Discipline;
  view: View;
  player: string;
  handedness: Handedness;
  source: string;
  /** browser: made on this page. legacy_csv: imported from the 2026 hackathon files (ADR 0008). */
  origin: "browser" | "legacy_csv";
  fps: number;
  frame_count: number;
  duration_s: number;
  created_at: string;
};

export type ReferenceWithPose = Reference & { pose_sequence: PoseSequence };

export type Gate = {
  shot: string;
  shot_name: string;
  discipline: Discipline;
  view: View;
  /** Browser references: these count toward the gate. */
  references: number;
  /** Old CSV references: shown, not counted. */
  legacy_references: number;
  lopo_passed: boolean;
  enabled: boolean;
};

export type Gates = { min_references: number; gates: Gate[] };

export type NewReference = {
  shot: string;
  view: View;
  player: string;
  handedness: Handedness;
  source: string;
  pose_sequence: PoseSequence;
};

async function fail(res: Response): Promise<never> {
  const body: unknown = await res.json().catch(() => null);
  const b = (typeof body === "object" && body) || {};
  const code = "error" in b ? String(b.error) : "unknown";
  throw new AnalysisError(res.status, code, "message" in b ? String(b.message) : undefined);
}

export async function createReference(ref: NewReference, csrf: string): Promise<{ id: string }> {
  const res = await fetch("/api/admin/references", {
    method: "POST",
    headers: { "Content-Type": "application/json", [CSRF_HEADER]: csrf },
    body: JSON.stringify(ref),
  });
  if (res.status !== 201) return fail(res);
  return res.json();
}

export async function listReferences(): Promise<Reference[]> {
  const res = await fetch("/api/admin/references", { headers: { Accept: "application/json" } });
  if (res.status !== 200) return fail(res);
  return ((await res.json()) as { references: Reference[] }).references;
}

export async function getReference(id: string): Promise<ReferenceWithPose> {
  const res = await fetch(`/api/admin/references/${encodeURIComponent(id)}`, { headers: { Accept: "application/json" } });
  if (res.status !== 200) return fail(res);
  return res.json();
}

export async function getShotGates(): Promise<Gates> {
  const res = await fetch("/api/admin/shot-gates", { headers: { Accept: "application/json" } });
  if (res.status !== 200) return fail(res);
  return res.json();
}

/** Only http(s) sources become links; anything else (a note, a javascript: URL) stays text. */
export function sourceLink(source: string): string | undefined {
  try {
    const u = new URL(source);
    return u.protocol === "https:" || u.protocol === "http:" ? u.href : undefined;
  } catch {
    return undefined;
  }
}
