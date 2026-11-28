# 0008. Import the old CSVs as legacy references that do not count toward a shot gate

- Status: accepted (owner, 2026-11-28)
- Date: 2026-11-28

## Context
The 2026 hackathon build left 12 pro pose CSVs in `reference_data/cricket/`. Slice 4 must import them, and every reference must be measured the same way as an athlete's clip (ADR 0001). The files were made outside the repo, by a script that no longer exists, so their format was measured, not assumed:

| Finding | Evidence |
|---|---|
| 13 joints, not 33: nose, shoulders, elbows, wrists, hips, knees, ankles; each with x, y, z, visibility | the header (54 columns) |
| x and y are MediaPipe **image** coordinates (fractions of width and height) with the hip midpoint subtracted in every frame | the hip midpoint is exactly (0, 0) in every row of all 12 files |
| They are **not** world coordinates in metres | body size differs 3.7 times between files (median nose-to-ankle 0.17 for Sachin, 0.63 for Starc; shin 0.08 to 0.38). World landmarks give about the same body size in every clip. |
| z is MediaPipe's rough image depth, not a world position | hip-midpoint z is about 0 but not exactly; nose z values such as -0.69 |
| The video size is lost | no width or height anywhere; the figures look right at 16:9 |
| Frame rates differ, with dropped frames | 15 fps (Bumrah, Rohit, Sachin, Kohli, ABD), 25 (Shami), 30 (the rest); missing frame numbers in 5 files |
| Two files are longer than the 10 s limit | Bhuvneshwar 14.8 s, Bumrah 12.2 s; in both, the delivery is at the end |
| Views (from stick-figure sheets): all 7 bowlers front-on; the 4 batters side-on | owner confirmed the tags |

Browser references (slice 3 code) have 33 landmarks, world coordinates in metres, a known video size and a uniform 30 fps.

## Decision
1. **Import them, marked as legacy.** `pro_references.origin` is `browser` or `legacy_csv`. A one-time command, `cmd/import-legacy`, converts each file with `internal/legacy` and stores it in the same compressed format (TBP1) as every other pose sequence. The owner's tags (player, shot, view, handedness) are in the command. `source` names the old file, and a unique partial index on it makes a second run add nothing.
2. **Mapping:** the 13 joints go to their MediaPipe slots (0, 11-16, 23-28). x and y get +0.5, so the hip midpoint sits in the middle of the picture. Visibility is kept. z is dropped. World values are 0. The 20 landmarks the files never had are all 0: "not measured", which the stick figure skips (a low-visibility landmark is still drawn dashed). Width × height is set to 1280 × 720 (assumed 16:9).
3. **Timing:** each file is resampled to a uniform grid at its own frame rate (the median step), with linear interpolation for dropped frames. A file longer than 10 s keeps its last 10 s.
4. **Not counted:** `shot_gates.reference_count` counts browser references only. The admin page shows old CSV references apart ("+6 old CSV, not counted"). They cannot be compared with browser references: no world coordinates, an assumed aspect ratio, no heel or foot points, a different MediaPipe version, and broadcast camera moves.
5. **Re-create the references from clips** on the admin page. The old files are kept in the database for the record.
6. After a successful import, `reference_data/` is deleted from the repo (owner's choice); git history keeps it.

## Options considered
- **Import and count them:** fills the gates sooner, but scores would mix two measurement systems. A leave-one-pro-out test might even pass on numbers that are not comparable.
- **Skip the import:** simplest, but loses the record of the hackathon data, and the stick figures are useful to check views and shots.
- **Guess the world coordinates from the image ones:** needs the camera and the body size; any guess would be a made-up number.

## Consequences
- No shot + view has an open gate from old data. Slice 5 (fast bowling, side-on) needs 3 or more side-on fast-bowling clips added on the admin page; all old bowling files are front-on anyway.
- Scoring code must check `origin` if it ever reads references outside a gate count.
- The import is idempotent and can run again on a new database, as long as the files exist (git history).
