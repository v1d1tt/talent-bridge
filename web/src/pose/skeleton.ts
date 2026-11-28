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

/**
 * A landmark with all six values 0 was not measured at all: the old CSV
 * references (ADR 0008) have only 13 of the 33 points. It is different
 * from a landmark the model could not see (low visibility), which is drawn
 * dashed.
 */
export function measured(l: Landmark): boolean {
  return l.some((v) => v !== 0);
}

/** The body segments to draw: those whose two ends were measured. */
export function drawnSegments(frame: Landmark[]): ReadonlyArray<readonly [number, number]> {
  return BODY.filter(([a, b]) => measured(frame[a]) && measured(frame[b]));
}

/** Where the head circle goes (fractions of the frame); the ears, when measured, set its size. */
export function head(frame: Landmark[]) {
  const [n, le, re] = [frame[0], frame[7], frame[8]];
  const fromEars = measured(le) && measured(re);
  return fromEars
    ? { x: (le[0] + re[0] + n[0]) / 3, y: (le[1] + re[1] + n[1]) / 3, fromEars, ears: [le[0], le[1], re[0], re[1]] }
    : { x: n[0], y: n[1], fromEars, ears: [0, 0, 0, 0] };
}

/** How the plate zooms: x and y move by (x - cx) * k + 0.5, the same k on both axes. */
export type View = { k: number; cx: number; cy: number };

/** Free space kept on each side of the figure, as a fraction of the plate. */
const MARGIN = 0.08;

/**
 * The zoom that makes the box around every measured point of the whole
 * sequence fill the plate. One view for all frames, so the figure keeps
 * its size and place while the slider moves. The box is cut to the video
 * frame: the plate never zooms out past the full video.
 */
export function fitView(frames: Landmark[][]): View {
  let [x0, y0, x1, y1] = [1, 1, 0, 0];
  for (const f of frames) {
    if (f.every((l) => l[VIS] === 0)) continue; // no person: drawSkeleton skips it too
    for (const l of f) {
      if (!measured(l)) continue;
      const x = Math.min(1, Math.max(0, l[0]));
      const y = Math.min(1, Math.max(0, l[1]));
      [x0, y0, x1, y1] = [Math.min(x0, x), Math.min(y0, y), Math.max(x1, x), Math.max(y1, y)];
    }
  }
  const span = Math.max(x1 - x0, y1 - y0);
  if (span <= 0) return { k: 1, cx: 0.5, cy: 0.5 };
  return { k: (1 - 2 * MARGIN) / span, cx: (x0 + x1) / 2, cy: (y0 + y1) / 2 };
}

/** One frame moved by a view. A not-measured landmark stays all 0. */
export function fitFrame(frame: Landmark[], v: View): Landmark[] {
  return frame.map((l) =>
    measured(l) ? [(l[0] - v.cx) * v.k + 0.5, (l[1] - v.cy) * v.k + 0.5, l[2], l[3], l[4], l[5]] : l,
  );
}

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
  const segments = drawnSegments(frame);

  if (ink.halo) {
    ctx.strokeStyle = ink.halo;
    ctx.lineWidth = lw * 2.6;
    ctx.beginPath();
    for (const [a, b] of segments) {
      ctx.moveTo(...px(frame[a]));
      ctx.lineTo(...px(frame[b]));
    }
    ctx.stroke();
  }
  for (const [a, b] of segments) {
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

  // Head: a circle through the ears, centred between them and the nose.
  const hd = head(frame);
  const r = hd.fromEars ? Math.max(Math.hypot((hd.ears[0] - hd.ears[2]) * w, (hd.ears[1] - hd.ears[3]) * h) / 2, lw * 3) : lw * 3;
  ctx.strokeStyle = frame[0][VIS] >= SEEN ? ink.line : ink.faint;
  ctx.lineWidth = lw;
  ctx.beginPath();
  ctx.arc(hd.x * w, hd.y * h, r, 0, Math.PI * 2);
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
