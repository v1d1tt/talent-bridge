// Draws one frame of a pose sequence as an ink stick figure (the
// "skeleton plate" of docs/design/ui-direction.md). Kept free of the
// MediaPipe package so the result page does not download it.
import { VIS, type Landmark } from "./types";

/**
 * Body segments of MediaPipe's 33-landmark pose, from the shoulders down.
 * The face points (0-10) are drawn as one head circle instead.
 */
export const BODY: ReadonlyArray<readonly [number, number]> = [
  [11, 12], [11, 23], [12, 24], [23, 24], // torso
  [11, 13], [13, 15], [15, 19], [12, 14], [14, 16], [16, 20], // arms, to the index finger
  [23, 25], [25, 27], [27, 31], [27, 29], [29, 31], // left leg and foot
  [24, 26], [26, 28], [28, 32], [28, 30], [30, 32], // right leg and foot
];

const SEEN = 0.5;

export type Ink = {
  line: string;
  faint: string;
  /** Paper-coloured outline under each stroke, so the ink reads over a video frame. */
  halo?: string;
};

/**
 * Draws the figure into the whole canvas. The canvas must have the video's
 * aspect ratio, because x and y are fractions of the video's width and
 * height. Segments the model could not see are drawn dashed in the faint
 * ink: in the scorer's book, a struck mark means "not measured".
 */
export function drawSkeleton(ctx: CanvasRenderingContext2D, frame: Landmark[] | undefined, ink: Ink) {
  const { width: w, height: h } = ctx.canvas;
  ctx.clearRect(0, 0, w, h);
  if (!frame || frame.every((l) => l[VIS] === 0)) return;
  const px = (l: Landmark) => [l[0] * w, l[1] * h] as const;
  const lw = Math.max(2, w / 160);
  ctx.lineCap = "round";
  ctx.lineJoin = "round";

  if (ink.halo) {
    ctx.strokeStyle = ink.halo;
    ctx.lineWidth = lw * 2.6;
    ctx.beginPath();
    for (const [a, b] of BODY) {
      ctx.moveTo(...px(frame[a]));
      ctx.lineTo(...px(frame[b]));
    }
    ctx.stroke();
  }
  for (const [a, b] of BODY) {
    const seen = frame[a][VIS] >= SEEN && frame[b][VIS] >= SEEN;
    ctx.strokeStyle = seen ? ink.line : ink.faint;
    ctx.lineWidth = seen ? lw : lw * 0.6;
    ctx.setLineDash(seen ? [] : [lw * 2, lw * 2]);
    ctx.beginPath();
    ctx.moveTo(...px(frame[a]));
    ctx.lineTo(...px(frame[b]));
    ctx.stroke();
  }
  ctx.setLineDash([]);

  // Head: a circle through the ears, centred between them.
  const [lx, ly] = px(frame[7]);
  const [rx, ry] = px(frame[8]);
  const [nx, ny] = px(frame[0]);
  const r = Math.max(Math.hypot(lx - rx, ly - ry) / 2, lw * 3);
  ctx.strokeStyle = frame[0][VIS] >= SEEN ? ink.line : ink.faint;
  ctx.lineWidth = lw;
  ctx.beginPath();
  ctx.arc((lx + rx + nx) / 3, (ly + ry + ny) / 3, r, 0, Math.PI * 2);
  ctx.stroke();

  // Joints as small filled dots.
  ctx.fillStyle = ink.line;
  for (const i of [11, 12, 13, 14, 15, 16, 23, 24, 25, 26, 27, 28]) {
    if (frame[i][VIS] < SEEN) continue;
    ctx.beginPath();
    ctx.arc(...px(frame[i]), lw * 0.9, 0, Math.PI * 2);
    ctx.fill();
  }
}

/** The inks, read from the page's CSS custom properties. */
export function inkFrom(el: Element): Ink {
  const s = getComputedStyle(el);
  return {
    line: s.getPropertyValue("--ink").trim() || "#1c2430",
    faint: s.getPropertyValue("--ink-muted").trim() || "#5b6573",
    halo: s.getPropertyValue("--paper").trim() || "#fbfbf8",
  };
}
