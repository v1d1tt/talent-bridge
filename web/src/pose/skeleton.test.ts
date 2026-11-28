import { describe, expect, it } from "vitest";
import { BODY, drawnSegments, head, measured } from "./skeleton";
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
