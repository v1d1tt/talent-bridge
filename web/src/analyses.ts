// The browser side of analyses: save the pose sequence, upload the key
// frames straight to Azure Blob Storage with the signed URLs the server
// returns, and read a saved analysis back.
import { RequestError } from "./me";
import type { PoseSequence } from "./pose/types";

const CSRF_HEADER = "X-CSRF-Token";

export type Upload = { frame_index: number; upload_url: string; expires_at: string };

export type KeyFrame = { frame_index: number; state: "pending" | "uploaded" | "rejected"; url?: string };

export type Analysis = {
  id: string;
  created_at: string;
  fps: number;
  frame_count: number;
  duration_s: number;
  pose_sequence: PoseSequence;
  score: number | null;
  key_frames_on: boolean;
  key_frames: KeyFrame[];
};

/** A refused request, with the server's plain-English reason when it gave one. */
export class AnalysisError extends RequestError {
  constructor(
    status: number,
    code: string,
    readonly reason?: string,
  ) {
    super(status, code);
    this.name = "AnalysisError";
  }
}

async function fail(res: Response): Promise<never> {
  const body: unknown = await res.json().catch(() => null);
  const b = (typeof body === "object" && body) || {};
  const code = "error" in b ? String(b.error) : "unknown";
  throw new AnalysisError(res.status, code, "message" in b ? String(b.message) : undefined);
}

export async function createAnalysis(
  seq: PoseSequence,
  keyFrames: number[],
  csrf: string,
): Promise<{ id: string; key_frames: Upload[] }> {
  const res = await fetch("/api/analyses", {
    method: "POST",
    headers: { "Content-Type": "application/json", [CSRF_HEADER]: csrf },
    body: JSON.stringify({ pose_sequence: seq, key_frames: keyFrames.map((frame_index) => ({ frame_index })) }),
  });
  if (res.status !== 201) return fail(res);
  return res.json();
}

/** PUTs one JPEG to its signed upload URL (Azure Blob Storage, Put Blob). */
export async function uploadKeyFrame(upload: Upload, jpeg: Blob): Promise<void> {
  const res = await fetch(upload.upload_url, {
    method: "PUT",
    headers: { "x-ms-blob-type": "BlockBlob", "Content-Type": "image/jpeg" },
    body: jpeg,
  });
  if (res.status !== 201) throw new AnalysisError(res.status, "upload_failed");
}

/** Asks the server to check the uploaded key frames. */
export async function checkKeyFrames(id: string, csrf: string): Promise<Analysis> {
  const res = await fetch(`/api/analyses/${encodeURIComponent(id)}/key-frames/check`, {
    method: "POST",
    headers: { [CSRF_HEADER]: csrf },
  });
  if (res.status !== 200) return fail(res);
  return res.json();
}

export async function getAnalysis(id: string): Promise<Analysis> {
  const res = await fetch(`/api/analyses/${encodeURIComponent(id)}`, { headers: { Accept: "application/json" } });
  if (res.status !== 200) return fail(res);
  return res.json();
}
