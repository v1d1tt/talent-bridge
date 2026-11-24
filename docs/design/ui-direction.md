# UI direction: the Scorer's book

- Status: chosen by the owner on 2026-11-25 (impeccable shape round, seed `c83d9718`, pick card).
- Product truth: [PRODUCT.md](../../PRODUCT.md). This file is the brief for the build. The full `DESIGN.md` (tokens and components) is written at the end of the build from what was actually built.

## Job and audience

- **First viewer: a recruiter on a laptop**, opening the live link or watching the demo. Within one screen they must see what the app does (clip → pose → score against pros → what to fix) and see that the numbers are real.
- **Athletes** check a result right after filming (phone) or review at home (phone or laptop). **Coaches** scan lists on a laptop. Mode: Operate. The task, state and next action always come before expression.

## The world

A cricket scorer's book. A scorer records every ball in ruled cells, with neat ink marks, and the totals are always visible. Talent-Bridge records every phase and joint the same way.

- **Paper and rule:** a light ruled paper ground; hairline rules form the grid of every table, list and form. No cards with shadows, no gradients, no glass.
- **Ink, not paint:** blue-black ink for print and data; red ink only for faults; blue ballpoint ink only for the coach's notes. Colour strategy: restrained (neutrals + 2 inks with fixed meanings).
- **Type:** a text serif with a matching Devanagari cut (sketch used Noto Serif + Noto Serif Devanagari) for all print. A handwriting face with Latin + Devanagari (sketch used Kalam) only for the coach's notes and fault marks. Numbers use tabular figures. Pick final faces in slice 1; both scripts at equal quality.
- **Marks as state:** `·` = measured and fine, a red `✗` + value = fault, empty cell = not measured yet, a struck-through cell = not measured (low visibility). Same marks in every screen.

## Structure across surfaces

| Surface | As a page of the book |
|---|---|
| Result | Left column: total score (large), closest pro, skeleton plate (athlete in ink over the pro in grey). Right: the sheet, phases as rows × metrics as columns, then "Fix first" coach note in ballpoint, then the rest of the faults. |
| Record / detect / confirm | The header of a new innings: shot, view, handedness written into the book's header fields; the athlete confirms the detected shot before the sheet opens. |
| History | Earlier pages of the book: one ruled row per session, trend drawn as a plain ink line. |
| Scouting list (coach) | A scorecard summary: one ruled row per athlete (name, shots, best score, region, age band); filters as the header row. |
| Admin references | The reference register: ruled rows per pro reference, with the shot-gate status per shot + view. |
| Drill plan | A practice log: 4 weeks as ruled columns, drills written in. |

## Signature interaction

When the server returns a score, the sheet fills in cell by cell (phase by phase), and fault marks are inked in red last. Then the total is written. This is the first-viewport demo moment for a recruiter. With reduced motion, the full sheet shows at once.

## States the build must design

First innings (empty book) · pose extraction in progress (frame counter, the skeleton plate draws live) · unclear view rejected (plain reason + how to film again) · shot gated off ("not enough pro references yet") · scoring in progress · coach agent fallback (template note, marked as such) · offline / server cold start · Hindi strings longer than English.

## Boundaries

- No pro broadcast footage anywhere. Reference plates show skeletons only.
- Sample data in demos is labelled "synthetic".
- Known limits (front-on accuracy, leave-one-pro-out only) appear next to the score in plain words, not hidden in a FAQ.
- Anti-goals: dark neon "sports analytics" dashboards, ball-tracking broadcast graphics, cream-and-serif "editorial" pastiche, gamified streaks and badges.

## Constraints

- SolidJS 2 + Vite, plain CSS with custom properties and nesting (no Tailwind).
- WCAG 2.2 AA; ink colours must pass contrast on the paper ground; red is never the only fault signal (the `✗` mark carries it too).
- English and Hindi from day 1; language switch in the header; all copy in message files.
- Phone first for athlete screens, laptop first for coach and admin screens.

## Open decisions (for the build, not to be invented later)

- Final font files and the exact ink and paper values (slice 1, checked against AA in both scripts).
- Hindi domain terms for the glossary words (owner to confirm the list in slice 1).
