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
- **Type (owner's choice, 2026-11-25, slice 1):** the device's own UI font for everything: `system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`. No serif and no handwriting face; nothing to download. Coach notes use the same font in ballpoint ink. Numbers use tabular figures.
- **Paper and inks (slice 1, contrast on the paper):**

  | Token | Value | Use | Contrast |
  |---|---|---|---|
  | `--paper` | `#FBFBF8` | page ground | — |
  | `--ink` | `#1C2430` | print, data, "connected" status | 15.1:1 |
  | `--ink-muted` | `#5B6573` | secondary text, struck cells | 5.7:1 |
  | `--ink-fault` | `#B42318` | faults only, always with `✗` | 6.3:1 |
  | `--ink-coach` | `#1D3FAF` | coach's notes, focus ring | 8.5:1 |
  | `--rule` | `#C8D4DD` | hairline rules (decorative) | 1.5:1 |
  | `--rule-strong` | `#77838F` | control borders | 3.7:1 |

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
| Settings (`/settings`, header link) | The back page of the book. Holds the system status sheet (server, database, schema, build). System status is not on the main page. "Connected" is marked in blue-black ink; there is no green anywhere. |

## Signature interaction

When the server returns a score, the sheet fills in cell by cell (phase by phase), and fault marks are inked in red last. Then the total is written. This is the first-viewport demo moment for a recruiter. With reduced motion, the full sheet shows at once.

## States the build must design

First innings (empty book) · pose extraction in progress (frame counter, the skeleton plate draws live) · unclear view rejected (plain reason + how to film again) · shot gated off ("not enough pro references yet") · scoring in progress · coach agent fallback (template note, marked as such) · offline / server cold start.

## Boundaries

- No pro broadcast footage anywhere. Reference plates show skeletons only.
- Sample data in demos is labelled "synthetic".
- Known limits (front-on accuracy, leave-one-pro-out only) appear next to the score in plain words, not hidden in a FAQ.
- Anti-goals: dark neon "sports analytics" dashboards, ball-tracking broadcast graphics, cream-and-serif "editorial" pastiche, gamified streaks and badges.

## Constraints

- SolidJS 2 + Vite, plain CSS with custom properties and nesting (no Tailwind).
- WCAG 2.2 AA; ink colours must pass contrast on the paper ground; red is never the only fault signal (the `✗` mark carries it too).
- English only for now.
- Phone first for athlete screens, laptop first for coach and admin screens.
