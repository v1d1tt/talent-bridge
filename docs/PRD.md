# Talent-Bridge v2: Product Requirements

- Status: approved for build (2026-11-25)
- Owner: Vidit Arora
- Words: see [GLOSSARY.md](../GLOSSARY.md). Decisions: [docs/adr/](adr/).

## 1. Problem and goal

Young cricketers outside academies have no cheap way to check their technique, and coaches have no cheap way to find them. The 2026 hackathon build tried this with 3 Python scripts. It compared one elbow angle with a mixed average, used an LSTM trained on made-up data, and had no pose step, API, UI, tests or deploy.

**Goal:** an athlete records a cricket clip and gets a score they can trust, computed against pro players for that exact shot and camera view, with clear faults and a drill plan. Athletes can choose to be found by coaches.

## 2. Users

| Role | Wants | Can do |
|---|---|---|
| Athlete (13+) | Know what to fix, see progress, be found | Analyse clips, see history, edit profile, turn discoverable on/off, accept or decline contact requests, report |
| Coach | Find talent by shot and score | See the scouting list, filter it, send contact requests, report |
| Admin (owner) | Good references, a safe list | Add references, see shot gate status, review reports, hide profiles, ban users |

## 3. Scope

### Must have
1. **Pose in the browser.** MediaPipe Pose Landmarker for Web turns the clip into a pose sequence on the device. The clip never leaves the device (ADR 0001).
2. **All cricket shots, never mixed.** Every batting shot and bowling type has its own reference set per view.
3. **Two views.** `side_on` and `front_on`, detected from the pose. Unclear clips are rejected with a clear message.
4. **Shot detection with confirm.** The server suggests the shot with a confidence. The athlete confirms or changes it before scoring.
5. **Scoring (ADR 0002).** Normalise → detect view → detect phases → DTW to each reference → per-phase metric deltas → score 0-100, closest pro, top 2-3 faults. Server only.
6. **Shot gate.** A shot + view is offered only with 3+ references and a passing leave-one-pro-out test.
7. **Coach agent.** Google ADK for Go v2 with a Gemini model. Typed tools: `get_faults`, `get_history`, `compare_to_pro`, `build_drill_plan`. It explains the faults and builds a 4-week drill plan from the fixed drill catalogue. Template fallback when the model fails or fails the grounding check.
8. **Eval harness in CI.** Fixed sessions; checks that every number in the agent's text matches a tool result, that drills come only from the catalogue, and that the fault order matches the scoring output.
9. **History.** Every analysis is stored; a trend chart per shot. (The LSTM forecast is dropped.)
10. **Stored per analysis:** pose sequence (compressed), metrics, score, faults, coaching text, 2-3 key frames in Cloud Storage. No video.
11. **Admin reference page.** Upload a pro clip, tag player, shot, view, handedness, source; the same browser pose code creates the reference. One-time import of the old CSVs.
12. **Auth (ADR 0004).** Google sign-in, server-side sessions, CSRF, roles checked on every route.
13. **Scouting (ADR 0005).** Discoverable opt-in for any age; guardian consent under 18; coach-only list; no contact details shown; contact requests; report; admin hide/ban.
14. **English and Hindi from day 1.** Every screen, error, coaching text and drill plan in both languages; a language switch in the header; all copy in message files; the coach agent answers in the user's language, and the eval harness runs in both.
15. **UI direction: the Scorer's book** ([docs/design/ui-direction.md](design/ui-direction.md)).

### Should have
- Scouting filters: shot, score range, region, age band.
- Stick-figure overlay on key frames.

### Will not do in v2
In-app chat · badminton and other sports · LSTM or any forecast · storing full video · a learned scoring model · admin verification of coaches · email verification of guardians.

## 4. Inputs and outputs

| In (from browser) | Out (to browser) |
|---|---|
| Pose sequence JSON: fps, frames[33 landmarks: x, y, visibility, world x/y/z] | Shot guess + confidence |
| Handedness, confirmed shot | Score 0-100, view, phases, metric table vs reference range |
| 2-3 key frame JPEGs (direct to Cloud Storage with a signed URL) | Closest pro, top faults, coaching text, drill plan |
| Profile fields, discoverable, guardian consent | History chart data, scouting list, contact requests |

Input limits: clip length 1-10 s, 15-60 fps, max 600 frames, one person. The API rejects other shapes with a 422 and a clear reason.

## 5. Stack (anti-default check, versions checked 2026-11-25)

```
Part: Backend language and HTTP
AI default: Node + Express (or FastAPI)
Options: Node/Express | Go net/http or chi | Kotlin + Ktor | Rust + Axum
Pick: Go 1.27, stdlib net/http (chi v5.5.0 only if routing needs grow)
Reason: typed, fast for the DTW loops, small Docker image, closes the "typed backend" gap. Hand-written DTW is a clear DSA story.
Ledger: new (Sports Insight Bot used FastAPI)
```
```
Part: Data access and migrations
AI default: Prisma
Options: Prisma | sqlc + goose | GORM | raw SQL
Pick: sqlc v1.31.1 + pgx v5.11.0 + goose v3.30.1
Reason: you write plain SQL and get type-safe Go; migrations are explicit files. Teaches SQL, ties to the DBMS course. Closes "SQL with migrations".
Ledger: new
```
```
Part: Database
AI default: Supabase
Options: Supabase | Neon Postgres | Cloud SQL | SQLite/Turso
Pick: Neon Postgres (free: 100 CU-hours/month, 1 GB per project, scale to zero)
Reason: plain Postgres with no BaaS magic, free when idle. Cloud SQL costs money even when idle.
Ledger: new
```
```
Part: Frontend
AI default: Next.js + React
Options: Next.js | React + Vite | SolidJS + Vite | Vue
Pick: SolidJS 2.0.3 (@solidjs/web 2.0.3, @solidjs/vite-plugin 3.0.1, @solidjs/router 2.0.1), Vite 8.3, TypeScript
Reason: owner's choice; signals with JSX, no SSR needed because Go serves the API. Solid 2.0 shipped 2026-11-17, so start on the current major. Risk: few examples; check every API in Context7.
Ledger: new (Sports Insight Bot used Svelte 5)
```
```
Part: Styling
AI default: Tailwind + shadcn/ui
Options: Tailwind | plain CSS with custom properties | CSS Modules | Open Props
Pick: plain CSS with custom properties and nesting
Reason: the Scorer's book direction is ruled grids, two inks and a paper ground; a small set of custom properties carries it, and utility classes would add nothing. Teaches modern CSS.
Ledger: Sports Insight Bot used plain CSS
```
```
Part: Pose model
AI default: MediaPipe on a Python server
Options: server MediaPipe | MediaPipe Web | MoveNet (TF.js) | RTMPose (ONNX)
Pick: @mediapipe/tasks-vision 1.2.3, Pose Landmarker (lite or full by benchmark in slice 3)
Reason: runs on the device, 33 landmarks with world coordinates, Apache 2.0 for the web SDK and the versioned full model.
Ledger: new
```
```
Part: Host
AI default: Vercel
Options: Vercel | Fly.io | GCP Cloud Run | AWS App Runner
Pick: GCP Cloud Run (one Docker image: Go binary + built Solid files), min instances 0; Cloud Storage for key frames; deploy via google-github-actions/deploy-cloudrun with Workload Identity Federation
Reason: closes "Docker" and "major cloud" gaps; free tier covers a portfolio app.
Ledger: new (Sports Insight Bot used Vercel)
```
```
Part: Auth
AI default: Clerk / NextAuth
Options: Clerk | Auth.js | own sessions + Google OIDC
Pick: own server-side sessions in Postgres + Google OIDC (PKCE)
Reason: closes "auth done properly"; every part can be explained.
Ledger: new
```
```
Part: Agent / LLM
AI default: OpenAI SDK + LangChain
Options: LangChain | direct HTTP loop | Genkit Go | Google ADK for Go v2
Pick: Google ADK for Go v2 (google.golang.org/adk/v2, v2.8.0, 2026-11-16) with a Gemini Flash model
Reason: fresh-tech-scan pick; typed function tools (functiontool.New) inside the Go API; fits Gemini and Cloud Run. Model ID is picked in slice 7 by a small eval of the current Flash models (IDs change often; verify on ai.google.dev at build time).
Ledger: new (Sports Insight Bot used OpenRouter)
```
```
Part: Tests and CI
AI default: Jest
Options: Jest | go test + Vitest 5.1 + Playwright 1.65
Pick: go test (table tests, benchmarks), Vitest 5.1.1, Playwright 1.65.1, leave-one-pro-out and agent eval as CI jobs; GitHub Actions required on main
Reason: one test tool per layer.
Ledger: Sports Insight Bot used Vitest + Playwright
```

**Visual feedback:** no Solid tool exists on npm (Agentation needs React; `sv-agentation` is Svelte only). The owner leaves notes as text or screenshots in chat in each UI slice's feedback round.

## 6. Data model (first draft; slices refine it)

| Table | Key columns |
|---|---|
| `users` | id, google_sub (unique), email, display_name, role (`athlete`/`coach`/`admin`), age_band, region, handedness, banned_at, created_at |
| `auth_sessions` | id_hash (pk), user_id, csrf_token, created_at, last_seen_at, expires_at |
| `shots` | id, discipline, code (unique), name |
| `shot_gates` | shot_id, view, reference_count, lopo_passed, lopo_run_at, enabled |
| `pro_references` | id, shot_id, view, player, handedness, source, pose_sequence (compressed), created_by, created_at |
| `analyses` | id, user_id, shot_id, view, detected_shot_id, detected_confidence, pose_sequence (compressed), metrics jsonb, score, faults jsonb, closest_reference_id, coaching jsonb, created_at |
| `key_frames` | id, analysis_id, phase, object_path |
| `drills` | id, code, name, description, fixes (fault codes) |
| `profiles` | user_id (pk), discoverable, bio, public_contact, hidden_at |
| `guardian_consents` | user_id, guardian_email, consented_at |
| `contact_requests` | id, coach_id, athlete_id, message, state, created_at, decided_at |
| `reports` | id, reporter_id, target_type, target_id, reason, state, created_at |

Indexes for scouting: `profiles(discoverable) where hidden_at is null`, `analyses(user_id, shot_id, created_at)`.

## 7. API outline

| Method + path | Role | Purpose |
|---|---|---|
| `GET /api/health` | any | liveness + DB ping |
| `GET /auth/google/start`, `GET /auth/google/callback`, `POST /auth/signout` | any | sign-in flow |
| `GET /api/me` | signed in | user, role, csrf token |
| `POST /api/analyses/detect` | athlete | pose sequence → view, shot guess, confidence |
| `POST /api/analyses` | athlete | confirmed shot + pose sequence → analysis + signed upload URLs for key frames |
| `GET /api/analyses/{id}` · `GET /api/analyses?shot=` | owner | result · history |
| `POST /api/analyses/{id}/coach` | owner | run the coach agent (or fallback) |
| `GET/PUT /api/profile` · `POST /api/profile/guardian-consent` | athlete | profile, discoverable, consent |
| `GET /api/scouting` | coach | filtered list |
| `POST /api/contact-requests` · `POST /api/contact-requests/{id}/{accept,decline,withdraw}` | coach / athlete | contact flow |
| `POST /api/reports` | signed in | report |
| `POST/GET /api/admin/references` · `GET /api/admin/shot-gates` · `POST /api/admin/{hide,ban}` | admin | admin tasks |

## 8. Risks

| Risk | Plan |
|---|---|
| Too few references (1-2 per batting shot today, none front-on) | Shot gate keeps them off; owner adds clips on the admin page |
| 2D front-on metrics are noisy | Gate per shot + view; README states it |
| Leave-one-pro-out does not prove amateur ranking | Stated in README as a known limit |
| Minors on a list for self-declared coaches | ADR 0005 safeguards; admin verification is the first follow-up if abuse appears |
| Solid 2 is 8 days old | Context7 for every API; pin versions |
| Cold starts (Cloud Run + Neon) | Clear loading state; warm-up call on page load |
| Gemini model IDs change | Model picked by eval in slice 7; ID in config, not code |

## 9. Done means

- Live on Cloud Run; URL in the README.
- CI required on `main`: Go lint + tests, Vitest, Playwright, leave-one-pro-out, agent eval.
- Every slice merged by its own PR; a learning note per slice in `docs/learning/`.
- State-machine pass and architecture pass done; review findings fixed.
- Demo video, screenshots, license step, ledger row updated.

## 10. Slices

Tracked as GitHub issues #1-#11 (issue number = slice number), each with a "Blocked by" line. Each slice ends deployed and demoable.

| # | Slice | Blocked by |
|---|---|---|
| 1 | Walking skeleton: Solid page → Go `/api/health` + DB ping → Neon (migration 1) → Docker → Cloud Run; CI; repo cleanup | — |
| 2 | Google sign-in, auth sessions, CSRF, roles | 1 |
| 3 | Browser pose: clip → pose sequence + stick figure → save analysis (no score yet) + key frames to Cloud Storage | 2 |
| 4 | Admin reference page + import of the old CSVs | 3 |
| 5 | Scoring for fast bowling side-on: view, phases, DTW, faults, score; leave-one-pro-out in CI; result screen | 4 |
| 6 | Shot detection + confirm; shot gate for all shots and views | 5 |
| 7 | Coach agent (ADK Go v2) + eval harness in CI + template fallback | 5 |
| 8 | History + trend chart | 5 |
| 9 | Athlete profile, discoverable, guardian consent | 2 |
| 10 | Scouting list + contact requests | 8, 9 |
| 11 | Report + admin hide/ban | 10 |

After the slices: state-machine pass, architecture pass, review, demo and docs, license, ledger.
