# Talent-Bridge

An athlete records a cricket clip, and Talent-Bridge scores the technique against pro players for that exact shot and camera view. It shows the faults that matter most and gives a drill plan. Athletes can choose to be found by coaches.

Talent-Bridge started as my first hackathon build (Feb 2026), which the team did not finish. This is the v2 rebuild.

**Status:** slices 1-2 of 11 are built: the walking skeleton (page → Go server → Postgres → Azure Container Apps, with CI) and Google sign-in with server-side sessions, CSRF protection and roles. Recording, pose and scoring come in the next slices (GitHub issues #3-#11).

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

| Route | Who may call it |
|---|---|
| `GET /api/health` | anyone |
| `GET /auth/google/start`, `GET /auth/google/callback` | anyone (rate-limited) |
| `GET /api/me` | anyone: the user, role and CSRF token, or `{"user": null}` |
| `POST /api/me/role` | signed in, CSRF token, only while no role is set |
| `POST /auth/signout` | signed in, CSRF token |

## Stack

| Part | Choice | Why |
|---|---|---|
| Frontend | SolidJS 2.0.3, `@solidjs/router` 2.0.1, Vite 8.3, TypeScript 6.0 | Signals and JSX with no virtual DOM; Go serves the API, so no SSR is needed |
| Styling | Plain CSS with custom properties and nesting, system UI font | The "Scorer's book" direction: ruled hairlines, paper and two inks ([docs/design/ui-direction.md](docs/design/ui-direction.md)) |
| Backend | Go 1.27, standard library `net/http` routing | Typed, fast, one small static binary |
| Auth | Own sessions in Postgres + Google OpenID Connect: `golang.org/x/oauth2` 0.38 (PKCE), `github.com/coreos/go-oidc/v3` 3.21 (ID token checks) | Every part can be explained and tested; no hosted auth service ([ADR 0004](docs/adr/0004-own-sessions-and-google-sign-in.md)) |
| Database | Postgres on Neon (prod), Postgres 18 in Docker (local and CI) | Plain Postgres; Neon scales to zero |
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
make check        # lint, Go tests, Vitest and the Playwright smoke test (starts a Postgres container)
```

Run the app locally, with sign-in through a fake Google (no Google account needed):

```bash
make fake-google                          # terminal 1: fake Google sign-in on http://localhost:9999
make run-local                            # terminal 2: Postgres in Docker + the app on http://localhost:8080
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

In production these come from GitHub: `DATABASE_URL`, `GOOGLE_CLIENT_SECRET` and `ADMIN_EMAILS` are Actions secrets, `GOOGLE_CLIENT_ID` is an Actions variable, and the deploy sets `PUBLIC_URL` from the app's own URL.

`make help` lists every command.

## Project layout

```
cmd/server/          main: config from env, JSON logs, clean shutdown
cmd/fake-google/     fake Google sign-in for local development and Playwright
internal/server/     the route table (every route with its access rule), static files, security headers
internal/auth/       Google sign-in (OIDC + PKCE), auth sessions, CSRF, the access guard
internal/auth/fakegoogle/  the fake OpenID Connect provider (tests and local only)
internal/health/     GET /api/health
internal/database/   pgx pool, goose migrations with an advisory lock
internal/store/      Go code generated by sqlc (do not edit)
db/migrations/       goose SQL migrations (embedded in the binary)
db/queries/          SQL queries, input for sqlc
web/                 SolidJS app (src/), Vitest tests, Playwright smoke tests (e2e/)
Dockerfile           three stages: Node build, Go build, distroless runtime
.github/workflows/   CI and deploy
docs/learning/       one learning note per slice
```

## CI and deploy

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs on every push and pull request:

1. **Lint and test** (required on `main`): golangci-lint, gofmt, `sqlc diff`, Go tests against a Postgres service container (including the route × role table, CSRF and ban tests), TypeScript type check, Vitest, then Playwright against the real Go server, which signs in through the fake Google.
2. **Deploy to Azure Container Apps** (pushes only, after tests pass): builds the image, pushes it to GitHub Container Registry (`ghcr.io`) and deploys a new revision. A push to `main` updates the live app. A push to a `slice/*` branch deploys the same image to a separate preview app (`talent-bridge-preview`) with its own URL, so a slice is checked live before merge. Azure trusts GitHub through OIDC (a managed identity with a federated credential), so no key is stored anywhere. The database URL, the Google client secret and the admin emails are GitHub Actions secrets that the deploy copies into Container Apps secrets. After a deploy, CI checks that `/api/health` is up and that `/auth/google/start` sends the browser to Google with PKCE, a state and a nonce.

How CI, Docker and Azure Container Apps fit together is explained for beginners in [docs/learning/01.md](docs/learning/01.md). How sign-in works, file by file, is in [docs/learning/02.md](docs/learning/02.md).

## Old code

The 2026 hackathon code (Python, OpenCV, an LSTM, Gemini) was removed in slice 1; the pro broadcast clips and model files were also removed from the git history. The 12 pro cricket pose CSV files in `reference_data/cricket/` stay: slice 4 imports them as references. (`reference_data/badminton/` is out of scope and is not used.)
