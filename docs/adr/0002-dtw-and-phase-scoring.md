# 0002. Scoring with DTW alignment and per-phase metric deltas

- Status: accepted
- Date: 2026-11-25

## Context
The old score was `10 - 1.5 * z` of one average elbow angle against one mean of all CSVs (batting and bowling mixed, plus a football file). It ignored timing, handedness, view and phases. Scores must now be trusted by coaches in a public scouting list.

## Decision
1. Normalise the pose sequence: mirror left-handers, scale by torso length, centre on the hips.
2. Detect the view (`side_on` / `front_on`) from shoulder and hip width. Reject unclear clips.
3. Detect the phases of the shot with rules on joint speed and foot height.
4. Align the athlete's joint-angle sequence to each reference of the same shot + view with DTW (hand-written dynamic programming in Go, Sakoe-Chiba band, benchmarked).
5. At each phase, compare each metric with the range across the reference set. Out-of-range metrics become faults, ranked by size times weight.
6. Score (0-100) = a documented mix of the DTW distance to the closest pro and the fault sizes. The formula and weights live in one Go package with table tests.
7. Shot detection: the reference set with the smallest mean DTW distance, with a confidence from the gap to the second set.
8. Shot gate: a shot + view goes live only with 3+ references and a passing leave-one-pro-out test (runs in CI).

Metric definitions (feet gap, front-knee angle, elbow angle, etc.) follow the PoseForge paper (arXiv 2608.05971, Aug 2026). Only the published definitions are used. PoseForge's code has no license, so none of it is copied.

## Options considered
- Better statistics only (z-score per phase and joint): simpler, but ignores rhythm and timing.
- A learned scoring model: needs hundreds of rated amateur clips that do not exist.
- DTW + phase deltas (picked): explainable, testable, works with few references.

## Consequences
- Front-on views measure some joints badly in 2D. The gate decides per shot + view.
- The only score test is leave-one-pro-out. It shows that pros score high; it does not prove that amateur scores are well ordered. That is a known limit, written in the README.
