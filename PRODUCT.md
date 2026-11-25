# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

SolidJS 2 + Vite + TypeScript frontend, served by a Go API on GCP Cloud Run. Pose extraction runs in the browser with MediaPipe Pose Landmarker. Full record: `docs/PRD.md` section 5.

## Users

- **Primary evaluator: a recruiter or hiring engineer** reviewing this as a portfolio project, mostly on a laptop, from the live link or the demo video. They spend minutes, not hours. They must understand what the app does and see it work quickly, and the craft must hold up when they click around.
- **Athletes (13+):** cricketers outside academies. They film a few balls or shots and check the result right away on a phone, or review it later at home on a phone or laptop. Both scenes matter equally.
- **Coaches:** scout for talent on a laptop. They filter discoverable athletes by shot and score and send contact requests.
- **Admin (owner):** adds pro references and handles reports.

## Product Purpose

An athlete records a cricket clip, and Talent-Bridge scores the technique against pro players for that exact shot and camera view. It shows the faults that matter most and builds a 4-week drill plan. Athletes can choose to be found by coaches. Success: the athlete knows exactly what to fix next, and a coach can find a promising player without seeing their contact details first.

## Positioning

The clip never leaves the device: pose extraction runs in the browser. Every score is computed on the server against named pro references of the same shot and view, aligned in time (DTW) and broken down by phase and joint. The coach agent may only phrase numbers that the scoring produced; an eval in CI checks this. A shot is offered only when its reference set passes a leave-one-pro-out test.

## Operating Context

- Clips are short (1-10 s), filmed side-on or front-on, one person in frame.
- Results are read in two scenes: a quick check right after filming, and a longer review with history and drill plan.
- Coaches compare many athletes in a list, then open a profile.
- Admin work: upload a pro clip, tag player, shot, view and handedness, and watch the shot-gate status.

## Capabilities and Constraints

- Shots: all cricket batting shots and bowling types, each with its own reference set per view. A shot is offered only with 3+ references and a passing leave-one-pro-out test.
- English only for now (UI, errors, coaching text, drill plan). Other languages are out of scope for v2.
- Stored per analysis: pose sequence, metrics, score, faults, coaching, 2-3 key frames. No video.
- Scouting: discoverable is opt-in for any age; guardian consent under 18; coaches never see contact details before an accepted contact request (ADR 0005).
- Known limits that must be stated, not hidden: front-on 2D metrics are less exact; the score test proves only that pros score high.
- Undecided: the exact list of batting shots shown at launch depends on how many references the owner adds.

## Brand Commitments

- Name: **Talent-Bridge**. No logo exists yet.
- Voice: an honest coach. Plain and direct, says what is wrong, why it matters and what to do. No hype, no fake praise, open about limits.

## Evidence on Hand

- 14 pro pose sequences in `reference_data/` (git history) from the hackathon: Bumrah, Steyn, Malinga, Starc, Shami, Bhuvneshwar, Jadeja, Rashid, Kohli, AB de Villiers, Rohit (pull), Sachin (straight drive), plus 2 badminton files (out of scope). Views are not tagged yet.
- No real athlete data, no users, no testimonials, no accuracy figures. Do not invent any. Demo data must be labelled synthetic.
- Pro broadcast footage must not be shown (no rights).

## Product Principles

1. Show the mechanism, not a claim: the athlete sees their own skeleton next to the pro's at the same phase.
2. One fix at a time: lead with the single most important fault, then the rest.
3. Honest numbers: every number traces to a measurement; limits are stated where they apply.
4. Privacy by default: the clip stays on the device, discoverable is off until chosen, contact details are never shown first.

## Accessibility & Inclusion

- WCAG 2.2 AA: contrast, large touch targets on phones, full keyboard use on laptops, reduced-motion support.
