# Talent-Bridge

An athlete records a cricket clip, and Talent-Bridge scores the technique against pro players for that exact shot and camera view. It shows the faults that matter most and gives a drill plan. Athletes can choose to be found by coaches.

Talent-Bridge started as my first hackathon build (Feb 2026), which the team did not finish. This is the v2 rebuild.

**Status:** slices 1-4 of 11 are built: the walking skeleton (page → Go server → Postgres → Azure Container Apps, with CI), Google sign-in with server-side sessions, CSRF protection and roles, **pose extraction in the browser** (a clip becomes a saved pose sequence with key frames, and the clip never leaves the device), and the admin's **reference register** with shot gates. Scoring comes in slice 5 (GitHub issues #5-#11).

**Live:** https://talent-bridge.livelyplant-beb9c42a.malaysiawest.azurecontainerapps.io · the Settings page ([/settings](https://talent-bridge.livelyplant-beb9c42a.malaysiawest.azurecontainerapps.io/settings)) shows the live system status. The app scales to zero, so the first visit after a quiet period can take a few seconds.

## What works now

- `GET /api/health` reports the server, the database connection, its latency and the schema (migration) version. It answers 503 when the database is down and never shows the database error to the client.
- The home page says what the app will do. The Settings page shows the system status as a ruled sheet and has a "Check again" button.
- On start, the server applies the database migrations (goose). A Postgres advisory lock makes two instances that start together take turns.
- **Sign in with Google** (OpenID Connect, authorization code flow with PKCE, `state` and `nonce`). The server verifies the ID token: signature against Google's keys (JWKS), issuer, audience and expiry.
- **Server-side auth sessions** in Postgres. The cookie (`__Host-tb_session`: HttpOnly, Secure, SameSite=Lax) holds a random 256-bit id; the database stores only its SHA-256 hash. A new id at every sign-in, idle expiry (7 days) and absolute expiry (30 days). Sign-out deletes the row.
- **CSRF token** per auth session: every POST/PUT/PATCH/DELETE without the right `X-CSRF-Token` header gets 403.
- **Roles:** at first sign-in a user chooses athlete or coach (once). Admin comes from the `ADMIN_EMAILS` setting, never from the repo. One guard checks every route; a route without an access rule is denied. A banned user cannot sign in, and all their sessions are deleted.
- The header shows who is signed in, their role, Settings and Sign out. Sign-in requests are rate-limited per client IP.
- **Pose in the browser** (`/analyses/new`, athletes): record or choose a 1-10 s clip. MediaPipe Pose Landmarker (WebAssembly, GPU or CPU) reads every 1/30 s of it on the device and draws the stick figure live over the video. **The clip never leaves the device**; a Playwright test records every request of the flow and fails if any carries video bytes.
- **Saved analysis:** the server checks the pose sequence (15-600 frames, 15-60 fps, 1-10 s, exactly 33 landmarks of 6 values, every value in range) and answers 422 with a reason otherwise. It stores the sequence compressed (fixed-point, delta-coded varints, DEFLATE) in Postgres. No score yet: scoring is slice 5.
- **Key frames:** the browser cuts 3 JPEG stills (for now the moments of most movement; phases come with scoring) and uploads them **straight to Azure Blob Storage** with short-lived signed URLs (user delegation SAS, create-only, 10 minutes). The server checks each upload (a JPEG of 100 B-512 KiB) and a sweep deletes anything else once the upload window closes. The result page (`/analyses/<id>`, owner only) shows the stick figure plate, the clip's numbers and the key frames through 15-minute read URLs.
- **Reference register** (`/admin/references`, admin only): the admin tags a pro clip (player, shot, side-on or front-on, handedness, source) and the browser reads it with the same pose code as an athlete's clip. Only the pose sequence is saved; no picture of a pro clip is stored or shown, and the page draws only the stick figure. The **shot gate** table shows the references per shot + view with a "needs 3+" mark; each reference has its own stick-figure page. The shot list (3 bowling types, 12 batting shots) is seeded by migration 4.
- **Old CSVs imported:** the 12 pro pose CSVs of the hackathon build were imported once (`cmd/import-legacy`). They turned out to be hip-centred image coordinates with 13 points, not 33, so they are kept as **old CSV references**: drawn, but not counted toward a shot gate ([ADR 0008](docs/adr/0008-import-old-csvs-as-legacy-references.md)).
- **Pose model benchmark** (`/lab/pose-benchmark`, linked from Settings): lite vs full on one clip, on the device. The app uses the full model; the numbers and the reason are in [docs/learning/03.md](docs/learning/03.md).

| Route | Who may call it |
|---|---|
| `GET /api/health` | anyone |
| `GET /auth/google/start`, `GET /auth/google/callback` | anyone (rate-limited) |
| `GET /api/me` | anyone: the user, role and CSRF token, or `{"user": null}` |
| `POST /api/me/role` | signed in, CSRF token, only while no role is set |
| `POST /auth/signout` | signed in, CSRF token |
| `POST /api/analyses` | athlete, CSRF token; 30 per day |
| `GET /api/analyses/{id}` | the athlete who owns it (anyone else: 404) |
| `POST /api/analyses/{id}/key-frames/check` | the owner, CSRF token |
| `POST /api/admin/references` | admin, CSRF token |
| `GET /api/admin/references`, `GET /api/admin/references/{id}`, `GET /api/admin/shot-gates` | admin |

## Stack

| Part | Choice | Why |
|---|---|---|
| Frontend | SolidJS 2.0.3, `@solidjs/router` 2.0.1, Vite 8.3, TypeScript 6.0 | Signals and JSX with no virtual DOM; Go serves the API, so no SSR is needed |
| Pose | MediaPipe Pose Landmarker for Web (`@mediapipe/tasks-vision` 1.2.3), served from the app's own origin under Apache 2.0 | Runs on the device, so no video is uploaded ([ADR 0001](docs/adr/0001-pose-extraction-in-the-browser.md)) |
| Styling | Plain CSS with custom properties and nesting, system UI font | The "Scorer's book" direction: ruled hairlines, paper and two inks ([docs/design/ui-direction.md](docs/design/ui-direction.md)) |
| Backend | Go 1.27, standard library `net/http` routing | Typed, fast, one small static binary |
| Auth | Own sessions in Postgres + Google OpenID Connect: `golang.org/x/oauth2` 0.38 (PKCE), `github.com/coreos/go-oidc/v3` 3.21 (ID token checks) | Every part can be explained and tested; no hosted auth service ([ADR 0004](docs/adr/0004-own-sessions-and-google-sign-in.md)) |
| Database | Postgres on Neon (prod), Postgres 18 in Docker (local and CI) | Plain Postgres; Neon scales to zero |
| Key frames | Azure Blob Storage, user delegation SAS (`azblob` 1.8, `azidentity` 1.15); Azurite 3.38 locally and in CI | The browser uploads straight to storage; no storage key exists on Azure |
| Data access | sqlc 1.31 (type-safe Go from plain SQL), pgx 5.13, goose 3.30 (migrations) | You write SQL; the compiler checks the Go |
| Host | Docker image on Azure Container Apps (Azure for Students), min replicas 0; image on ghcr.io | One image holds the binary and the built page; scales to zero. Moved from GCP Cloud Run because GCP billing setup failed ([ADR 0006](docs/adr/0006-azure-container-apps-host.md)) |
| Tests | `go test`, Vitest 5.1, Playwright 1.65 | One tool per layer |
| CI/CD | GitHub Actions; deploy to Azure with OIDC (no stored keys) | Every push is tested; only `main` gets traffic |

The full plan is in [docs/PRD.md](docs/PRD.md); the words are defined in [GLOSSARY.md](GLOSSARY.md); decisions are in [docs/adr/](docs/adr/).

## Run it from a fresh clone

You need Go 1.27, Node 24+, Docker, and (for `make generate` only) sqlc.

```bash
git clone https://github.com/v1d1tt/talent-bridge.git
cd talent-bridge
(cd web && npm ci && npx playwright install chromium)
make check        # lint, Go tests, Vitest and Playwright (starts Postgres and Azurite containers)
```

Run the app locally, with sign-in through a fake Google (no Google account needed):

```bash
make fake-google                          # terminal 1: fake Google sign-in on http://localhost:9999
make run-local                            # terminal 2: Postgres and Azurite in Docker + the app on http://localhost:8080
```

Open http://localhost:8080, click **Sign in**, then **Sign in with Google**. The fake shows a form: type any id, email and name. `admin@example.com` gets the admin role. The server trusts the fake only because both run on localhost; a deployed app refuses any issuer but Google ([ADR 0007](docs/adr/0007-fake-google-for-local-sign-in.md)).

To sign in with real Google locally, put these lines in `.env` (git-ignored) and run `make run`:

```
DATABASE_URL=postgres://postgres:postgres@localhost:54329/talent_bridge?sslmode=disable
GOOGLE_CLIENT_ID=...apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=...
ADMIN_EMAILS=you@gmail.com
```

The Google OAuth client must list `http://localhost:8080/auth/google/callback` as a redirect URI. Without `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`, the app runs with sign-in off. To use Neon instead of Docker, put its direct (not pooled) connection string in `DATABASE_URL`.

For frontend work with hot reload, run the server with `PUBLIC_URL=http://localhost:5173` and `make dev` in another terminal, then open http://localhost:5173 (Vite sends `/api` and `/auth` to the Go server).

Build and run the production image:

```bash
make docker
docker run -p 8080:8080 -e DATABASE_URL='postgres://postgres:postgres@host.docker.internal:54329/talent_bridge?sslmode=disable' talent-bridge:local
```

### Settings (environment variables)

| Variable | Needed | Meaning |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | for sign-in | the Google OAuth web client |
| `PUBLIC_URL` | for sign-in | the app's origin; the Google redirect URI is `PUBLIC_URL/auth/google/callback` (default `http://localhost:8080`) |
| `ADMIN_EMAILS` | no | comma-separated emails that get the admin role (Google must say the email is verified) |
| `GOOGLE_ISSUER` | local only | the fake Google's URL; refused unless it and `PUBLIC_URL` are on localhost |
| `BLOB_ACCOUNT_URL` | for key frames | `https://<account>.blob.core.windows.net` (or Azurite's `http://127.0.0.1:10000/devstoreaccount1`); without it, analyses are saved without key frames |
| `BLOB_CONTAINER` | no | default `key-frames` |
| `BLOB_IDENTITY_CLIENT_ID` | on Azure | client id of the app's user-assigned managed identity, which signs the SAS URLs |
| `BLOB_ACCOUNT_KEY` | local only | Azurite's public development key; refused unless the storage URL and `PUBLIC_URL` are on localhost |

In production these come from GitHub: `DATABASE_URL`, `GOOGLE_CLIENT_SECRET` and `ADMIN_EMAILS` are Actions secrets, `GOOGLE_CLIENT_ID` and `AZURE_STORAGE_ACCOUNT` are Actions variables, and the deploy sets `PUBLIC_URL` from the app's own URL and `BLOB_*` from the storage account and the `talent-bridge-app` identity.

The storage account was set up once with the Azure CLI: Standard LRS in the app's region, public blob access off, **shared key access off**, a private `key-frames` container, a CORS rule that lets only the live and preview origins `PUT`, and a user-assigned identity `talent-bridge-app` with `Storage Blob Delegator` on the account and `Storage Blob Data Contributor` on the container only.

`make help` lists every command.

## Project layout

```
cmd/server/          main: config from env, JSON logs, clean shutdown
cmd/fake-google/     fake Google sign-in for local development and Playwright
cmd/dev-blob/        prepares Azurite (container + CORS) for local runs and Playwright
internal/server/     the route table (every route with its access rule), static files, security headers
internal/auth/       Google sign-in (OIDC + PKCE), auth sessions, CSRF, the access guard
internal/auth/fakegoogle/  the fake OpenID Connect provider (tests and local only)
internal/health/     GET /api/health
internal/pose/       the pose sequence: shape and range checks, the compact stored format
internal/analysis/   POST/GET analyses, the key frame check and sweep
internal/reference/  the admin's references and shot gates (add, list, get)
internal/legacy/     converts the 2026 hackathon pose CSVs (ADR 0008)
cmd/import-legacy/   the one-time import of those CSVs (already run)
internal/blob/       Azure Blob Storage: signed upload and read URLs, inspect, delete
internal/database/   pgx pool, goose migrations with an advisory lock
internal/store/      Go code generated by sqlc (do not edit)
db/migrations/       goose SQL migrations (embedded in the binary)
db/queries/          SQL queries, input for sqlc
web/                 SolidJS app (src/; src/pose/ reads clips), Vitest tests, Playwright tests (e2e/)
web/scripts/         copies the MediaPipe files and models into the build (with their Apache 2.0 notice)
Dockerfile           three stages: Node build, Go build, distroless runtime
.github/workflows/   CI and deploy
docs/learning/       one learning note per slice
```

## CI and deploy

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs on every push and pull request:

1. **Lint and test** (required on `main`): golangci-lint, gofmt, `sqlc diff`, Go tests against a Postgres service container and Azurite (including the route × role table, CSRF, ban, 422 and owner tests), TypeScript type check, Vitest, then Playwright against the real Go server, which signs in through the fake Google and reads a real clip with the real pose model.
2. **Deploy to Azure Container Apps** (pushes only, after tests pass): builds the image, pushes it to GitHub Container Registry (`ghcr.io`) and deploys a new revision. A push to `main` updates the live app. A push to a `slice/*` branch deploys the same image to a separate preview app (`talent-bridge-preview`) with its own URL, so a slice is checked live before merge. Azure trusts GitHub through OIDC (a managed identity with a federated credential), so no key is stored anywhere. The database URL, the Google client secret and the admin emails are GitHub Actions secrets that the deploy copies into Container Apps secrets. After a deploy, CI checks that `/api/health` is up and that `/auth/google/start` sends the browser to Google with PKCE, a state and a nonce.

How CI, Docker and Azure Container Apps fit together is explained for beginners in [docs/learning/01.md](docs/learning/01.md). How sign-in works, file by file, is in [docs/learning/02.md](docs/learning/02.md). How a clip becomes a saved analysis, the lite vs full benchmark, the stored format and SAS vs proxy are in [docs/learning/03.md](docs/learning/03.md). The reference register, the shot-gate count under a row lock and how the old CSVs were measured are in [docs/learning/04.md](docs/learning/04.md).

## Third-party files

The app serves MediaPipe's WebAssembly runtime and the Pose Landmarker models (lite and full, float16 v1) unchanged under the Apache License 2.0; the licence and a notice are served with them at `/mediapipe/LICENSE` and `/mediapipe/NOTICE` ([web/third_party/mediapipe/](web/third_party/mediapipe/)). The Playwright test clip (`web/e2e/fixtures/side-on.webm`) is a 2 s cut of a bowling clip the owner filmed, used with the bowler's consent.

## Old code

The 2026 hackathon code (Python, OpenCV, an LSTM, Gemini) was removed in slice 1; the pro broadcast clips and model files were also removed from the git history. The 12 pro cricket pose CSVs in `reference_data/cricket/` are imported by `cmd/import-legacy` as old CSV references (ADR 0008); after the import they are removed from the repo (with the 2 out-of-scope badminton CSVs), and git history keeps them.
