import { describe, expect, it } from "vitest";
import { pickKeyFrames } from "./keyframes";
import { LANDMARKS, type Landmark } from "./types";

/** n frames; the body moves by `speed(f)` metres between frame f-1 and f. */
function clip(n: number, speed: (f: number) => number, visible = 0.9): Landmark[][] {
  const frames: Landmark[][] = [];
  let x = 0;
  for (let f = 0; f < n; f++) {
    x += f === 0 ? 0 : speed(f);
    frames.push(Array.from({ length: LANDMARKS }, (_, l): Landmark => [0.5, 0.5, visible, x, l * 0.01, 0]));
  }
  return frames;
}

describe("pickKeyFrames", () => {
  it("picks the frames with the most movement, spaced apart", () => {
    // Three bursts of movement around frames 20, 50 and 80.
    const burst = (f: number) => [20, 50, 80].reduce((s, c) => s + Math.max(0, 1 - Math.abs(f - c) / 4), 0) * 0.05;
    const picked = pickKeyFrames(clip(100, burst), 3);
    expect(picked).toHaveLength(3);
    expect(picked[0]).toBeGreaterThanOrEqual(18);
    expect(picked[0]).toBeLessThanOrEqual(22);
    expect(picked[1]).toBeGreaterThanOrEqual(48);
    expect(picked[1]).toBeLessThanOrEqual(52);
    expect(picked[2]).toBeGreaterThanOrEqual(78);
    expect(picked[2]).toBeLessThanOrEqual(82);
  });

  it("never picks two frames close together, even if one burst is biggest", () => {
    const burst = (f: number) => (Math.abs(f - 30) < 6 ? 0.2 : 0.001);
    const picked = pickKeyFrames(clip(90, burst), 3);
    for (let i = 1; i < picked.length; i++) expect(picked[i] - picked[i - 1]).toBeGreaterThanOrEqual(10);
  });

  it("falls back to evenly spaced frames when nothing moves", () => {
    expect(pickKeyFrames(clip(60, () => 0), 3)).toEqual([15, 30, 45]);
  });

  it("ignores landmarks the model could not see", () => {
    expect(pickKeyFrames(clip(60, () => 0.3, 0), 2)).toEqual([20, 40]);
  });

  it("returns sorted, distinct indexes inside the clip", () => {
    const picked = pickKeyFrames(clip(15, (f) => f * 0.01), 3);
    expect(new Set(picked).size).toBe(3);
    expect([...picked].sort((a, b) => a - b)).toEqual(picked);
    for (const i of picked) expect(i >= 0 && i < 15).toBe(true);
  });
});
