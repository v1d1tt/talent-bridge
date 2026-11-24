# 0003. Go API, Postgres on Neon, Docker on GCP Cloud Run

- Status: accepted
- Date: 2026-11-25

## Context
The app needs auth, a relational model (users, analyses, references, contact requests, reports), CPU work (DTW), and an agent. The portfolio has open gaps: a typed backend language, SQL with migrations, Docker and a major cloud.

## Decision
- Go 1.27 (stdlib `net/http`, or `chi` v5.5 if routing needs grow: decided in slice 1), `pgx` v5.11, `sqlc` v1.31 (type-safe code from plain SQL), `goose` v3.30 (migrations).
- Postgres on Neon free plan (100 CU-hours/month, 1 GB per project, scale to zero after 5 minutes).
- One Docker image (Go binary + built Solid files) on Cloud Run, min instances 0. Key frames in a Cloud Storage bucket, uploaded and read with signed URLs.
- CI on GitHub Actions; deploy with `google-github-actions/deploy-cloudrun` using Workload Identity Federation (no JSON keys).

## Options considered
- Python + FastAPI: reuses old code but closes no portfolio gap.
- Kotlin + Ktor, Rust + Axum: typed too, but slower builds or a steeper curve.
- Cloud SQL instead of Neon: closer to company setups, but costs money even when idle.

## Consequences
- Cold starts on Cloud Run and on Neon add delay to the first request. The UI must show a clear loading state.
- Free-tier limits: 1 GB of Postgres storage, so the full pose sequence is stored compressed, and old raw data may be pruned.
