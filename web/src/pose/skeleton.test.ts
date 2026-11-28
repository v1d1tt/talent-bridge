import { describe, expect, it } from "vitest";
import { BODY, drawnSegments, fitFrame, fitView, head, measured } from "./skeleton";
import { LANDMARKS, type Landmark } from "./types";

const seen = (x: number, y: number): Landmark => [x, y, 0.9, 0.1, 0.1, 0.1];
const NOT_MEASURED: Landmark = [0, 0, 0, 0, 0, 0];

// A frame like an imported old CSV (ADR 0008): only nose, shoulders, elbows,
// wrists, hips, knees and ankles; the other 20 landmarks are all 0.
const OLD_CSV_SLOTS = [0, 11, 12, 13, 14, 15, 16, 23, 24, 25, 26, 27, 28];
const oldCsvFrame = (): Landmark[] =>
  Array.from({ length: LANDMARKS }, (_, l) => (OLD_CSV_SLOTS.includes(l) ? seen(0.5, 0.2 + l / 50) : NOT_MEASURED));

describe("not measured landmarks", () => {
  it("all six values 0 means not measured; a low visibility does not", () => {
    expect(measured(NOT_MEASURED)).toBe(false);
    expect(measured([0.4, 0.5, 0, 0, 0, 0])).toBe(true); // a browser landmark the model could not see
    expect(measured(seen(0.4, 0.5))).toBe(true);
  });

  it("an old CSV frame draws no segment to a missing hand or foot point", () => {
    const segs = drawnSegments(oldCsvFrame());
    expect(segs.length).toBeGreaterThan(0);
    for (const [a, b] of segs) {
      expect(OLD_CSV_SLOTS).toContain(a);
      expect(OLD_CSV_SLOTS).toContain(b);
    }
    // Torso, arms to the wrist, legs to the ankle: 4 + 4 + 4 segments.
    expect(segs).toHaveLength(12);
  });

  it("a browser frame draws every segment", () => {
    const frame = Array.from({ length: LANDMARKS }, (_, l) => seen(0.3 + l / 100, 0.2 + l / 50));
    expect(drawnSegments(frame)).toEqual(BODY);
  });

  it("the head sits on the nose when the ears were not measured", () => {
    const frame = oldCsvFrame();
    const h = head(frame);
    expect(h.x).toBeCloseTo(frame[0][0]);
    expect(h.y).toBeCloseTo(frame[0][1]);
    expect(h.fromEars).toBe(false);
  });
});

describe("the plate zooms to fit", () => {
  // The box around the measured points of every frame of a sequence.
  const box = (frames: Landmark[][]) => {
    const pts = frames.flat().filter(measured);
    const xs = pts.map((l) => l[0]);
    const ys = pts.map((l) => l[1]);
    return { x0: Math.min(...xs), x1: Math.max(...xs), y0: Math.min(...ys), y1: Math.max(...ys) };
  };
  // A small figure, like an old CSV taken from a wide broadcast shot.
  const small = (dx: number): Landmark[] =>
    Array.from({ length: LANDMARKS }, (_, l) =>
      OLD_CSV_SLOTS.includes(l) ? seen(0.45 + dx + (l % 3) * 0.02, 0.4 + l / 200) : NOT_MEASURED,
    );

  it("the whole movement fills the plate, centred, with the same zoom on both axes", () => {
    const frames = [small(0), small(0.05)];
    const before = box(frames);
    const v = fitView(frames);
    const after = box(frames.map((f) => fitFrame(f, v)));
    const longest = Math.max(after.x1 - after.x0, after.y1 - after.y0);
    expect(longest).toBeCloseTo(0.84);
    expect((after.x0 + after.x1) / 2).toBeCloseTo(0.5);
    expect((after.y0 + after.y1) / 2).toBeCloseTo(0.5);
    expect((after.x1 - after.x0) / (after.y1 - after.y0)).toBeCloseTo((before.x1 - before.x0) / (before.y1 - before.y0));
  });

  it("one view for all frames: a still figure keeps its size while the slider moves", () => {
    const frames = [small(0), small(0.05)];
    const v = fitView(frames);
    const [a, b] = frames.map((f) => box([fitFrame(f, v)]));
    expect(a.x1 - a.x0).toBeCloseTo(b.x1 - b.x0);
    expect(a.x0).not.toBeCloseTo(b.x0); // and it still moves across the plate
  });

  it("not-measured landmarks stay all 0, so they are still skipped", () => {
    const f = small(0);
    const out = fitFrame(f, fitView([f]));
    for (let l = 0; l < LANDMARKS; l++) {
      if (!OLD_CSV_SLOTS.includes(l)) expect(out[l]).toEqual(NOT_MEASURED);
    }
    expect(drawnSegments(out)).toHaveLength(12);
  });

  it("frames with no person do not change the zoom", () => {
    const noPerson: Landmark[] = Array.from({ length: LANDMARKS }, () => [0.01, 0.99, 0, 0, 0, 0]);
    expect(fitView([small(0), noPerson])).toEqual(fitView([small(0)]));
  });

  it("never zooms out past the full video, and leaves an empty sequence as it is", () => {
    const wild: Landmark[] = Array.from({ length: LANDMARKS }, (_, l) => seen(-0.5 + l / 16, 1.4 - l / 20));
    const v = fitView([wild]);
    expect(v.k).toBeCloseTo(0.84);
    expect(v.cx).toBeCloseTo(0.5);
    expect(v.cy).toBeCloseTo(0.5);
    expect(fitView([])).toEqual({ k: 1, cx: 0.5, cy: 0.5 });
    expect(fitView([Array.from({ length: LANDMARKS }, () => NOT_MEASURED)])).toEqual({ k: 1, cx: 0.5, cy: 0.5 });
  });
});
