// Key frames for now: the moments of most movement. Phase detection (for
// example "release" or "contact") comes with scoring in slice 5; until then
// the page says how these frames were picked.
import { VIS, type Landmark } from "./types";

const SEEN = 0.5; // a landmark counts only if the model saw it in both frames

/**
 * Picks `count` frame indexes, in time order: the peaks of body movement
 * (summed world-space speed of the visible landmarks, lightly smoothed),
 * at least a fraction of the clip apart. If nothing moves, the frames are
 * evenly spaced.
 */
export function pickKeyFrames(frames: Landmark[][], count: number): number[] {
  const n = frames.length;
  const even = Array.from({ length: count }, (_, k) => Math.round(((k + 1) * n) / (count + 1)));
  if (n < count) return even.slice(0, n);

  const motion = new Array<number>(n).fill(0);
  for (let f = 1; f < n; f++) {
    let sum = 0;
    const a = frames[f - 1];
    const b = frames[f];
    for (let l = 0; l < b.length; l++) {
      if (a[l][VIS] < SEEN || b[l][VIS] < SEEN) continue;
      sum += Math.hypot(b[l][3] - a[l][3], b[l][4] - a[l][4], b[l][5] - a[l][5]);
    }
    motion[f] = sum;
  }
  // A 5-frame moving average, so one noisy frame is not a peak.
  const smooth = motion.map((_, f) => {
    let s = 0;
    let k = 0;
    for (let g = Math.max(0, f - 2); g <= Math.min(n - 1, f + 2); g++, k++) s += motion[g];
    return s / k;
  });
  if (Math.max(...smooth) < 1e-6) return even;

  // Greedy: take the highest frame, block its neighbours, repeat.
  const gap = Math.max(1, Math.floor(n / (count * 2)));
  const blocked = new Array<boolean>(n).fill(false);
  const picked: number[] = [];
  const order = smooth.map((v, f) => [v, f] as const).sort((x, y) => y[0] - x[0] || x[1] - y[1]);
  for (const [, f] of order) {
    if (picked.length === count) break;
    if (blocked[f]) continue;
    picked.push(f);
    for (let g = Math.max(0, f - gap + 1); g < Math.min(n, f + gap); g++) blocked[g] = true;
  }
  // A short clip can block everything before `count` frames are found.
  for (const f of even) if (picked.length < count && !picked.includes(f)) picked.push(f);
  return picked.sort((x, y) => x - y);
}
