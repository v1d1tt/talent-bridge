// Reads a clip frame by frame on the device and builds the pose sequence.
// The video file is opened from a local object URL and never uploaded:
// only the numbers (and later 2-3 small JPEG stills) leave the device.
import type { Loaded } from "./landmarker";
import { LANDMARKS, type Landmark, type PoseSequence } from "./types";

/** Frames read per second of clip. A fixed rate keeps every clip comparable. */
export const SAMPLE_FPS = 30;
export const MIN_SECONDS = 1;
export const MAX_SECONDS = 10;

export class ClipError extends Error {
  constructor(
    readonly code: "not_video" | "too_short" | "too_long" | "no_person",
    readonly seconds = 0,
  ) {
    super(code);
    this.name = "ClipError";
  }
}

/** How long opening a clip may take before it counts as not playable. */
const OPEN_TIMEOUT_MS = 20_000;

/** Waits until `ready()` is true (or `event` fires), or fails on error or time limit. */
function waitFor(video: HTMLVideoElement, event: string, ready: () => boolean): Promise<void> {
  if (ready()) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const done = (err?: ClipError) => {
      clearTimeout(timer);
      video.removeEventListener(event, ok);
      video.removeEventListener("error", bad);
      if (err) reject(err);
      else resolve();
    };
    const ok = () => done();
    const bad = () => done(new ClipError("not_video"));
    const timer = setTimeout(bad, OPEN_TIMEOUT_MS);
    video.addEventListener(event, ok);
    video.addEventListener("error", bad);
  });
}

/** Opens a picked file in a hidden <video>, ready to seek. */
export async function openClip(file: File): Promise<HTMLVideoElement> {
  const video = document.createElement("video");
  video.muted = true;
  video.playsInline = true;
  video.preload = "auto";
  video.src = URL.createObjectURL(file);
  // iOS Safari ignores preload and loads no frame data (no "loadeddata")
  // until the video plays. So: wait for the metadata only, then a muted
  // play and pause loads the first frame and unlocks drawing. Elsewhere
  // this is harmless. A time limit turns any other stall into an error.
  await waitFor(video, "loadedmetadata", () => video.readyState >= HTMLMediaElement.HAVE_METADATA);
  await video.play().then(
    () => video.pause(),
    () => undefined,
  );
  await waitFor(video, "loadeddata", () => video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA);
  const s = video.duration;
  if (!Number.isFinite(s) || video.videoWidth === 0) throw new ClipError("not_video");
  if (s < MIN_SECONDS) throw new ClipError("too_short", s);
  if (s > MAX_SECONDS + 0.05) throw new ClipError("too_long", s);
  return video;
}

/** Releases the object URL of a clip opened with openClip. */
export function closeClip(video: HTMLVideoElement) {
  URL.revokeObjectURL(video.src);
  video.removeAttribute("src");
  video.load();
}

export function seek(video: HTMLVideoElement, seconds: number): Promise<void> {
  return new Promise((resolve) => {
    if (Math.abs(video.currentTime - seconds) < 1e-4 && video.readyState >= 2) return resolve();
    video.addEventListener("seeked", () => resolve(), { once: true });
    video.currentTime = seconds;
  });
}

export type FrameProgress = { done: number; total: number; frame: Landmark[] };

export type Extracted = {
  sequence: PoseSequence;
  /** Model time per frame, in ms (for the benchmark). */
  msPerFrame: number[];
};

const r5 = (v: number) => Math.round(v * 1e5) / 1e5;
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const empty = (): Landmark[] => Array.from({ length: LANDMARKS }, (): Landmark => [0, 0, 0, 0, 0, 0]);

/**
 * Runs the pose model on every 1/30 s of the clip, in order. Values are
 * rounded to 5 decimals and kept inside the ranges the server accepts
 * (MediaPipe sometimes guesses an off-screen hand far outside the frame).
 */
export async function extractPose(
  video: HTMLVideoElement,
  model: Loaded,
  onFrame: (p: FrameProgress) => void,
  signal?: AbortSignal,
): Promise<Extracted> {
  const total = Math.max(1, Math.min(Math.floor(video.duration * SAMPLE_FPS), MAX_SECONDS * SAMPLE_FPS));
  const frames: Landmark[][] = [];
  const msPerFrame: number[] = [];
  for (let i = 0; i < total; i++) {
    signal?.throwIfAborted();
    await seek(video, i / SAMPLE_FPS);
    const t0 = performance.now();
    const res = model.landmarker.detectForVideo(video, model.nextTimestampMs);
    msPerFrame.push(performance.now() - t0);
    model.nextTimestampMs += 1000 / SAMPLE_FPS;

    const img = res.landmarks[0];
    const world = res.worldLandmarks[0];
    const frame =
      img && world
        ? img.map((p, k): Landmark => [
            r5(clamp(p.x, -0.5, 1.5)),
            r5(clamp(p.y, -0.5, 1.5)),
            r5(clamp(p.visibility ?? 0, 0, 1)),
            r5(clamp(world[k].x, -3, 3)),
            r5(clamp(world[k].y, -3, 3)),
            r5(clamp(world[k].z, -3, 3)),
          ])
        : empty();
    frames.push(frame);
    onFrame({ done: i + 1, total, frame });
  }
  const seen = frames.filter((f) => f.some((l) => l[2] > 0)).length;
  if (seen * 2 < frames.length) throw new ClipError("no_person");
  return { sequence: { fps: SAMPLE_FPS, width: video.videoWidth, height: video.videoHeight, frames }, msPerFrame };
}

/** Cuts one frame of the clip as a JPEG, at most `maxWidth` px wide. */
export async function cutJpeg(video: HTMLVideoElement, frameIndex: number, maxWidth = 640): Promise<Blob> {
  await seek(video, frameIndex / SAMPLE_FPS);
  const scale = Math.min(1, maxWidth / video.videoWidth);
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(video.videoWidth * scale);
  canvas.height = Math.round(video.videoHeight * scale);
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no 2d canvas");
  ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("JPEG encode failed"))), "image/jpeg", 0.85),
  );
}
