# 0006. Host on Azure Container Apps instead of GCP Cloud Run

- Status: accepted (replaces the host part of [ADR 0003](0003-go-postgres-cloud-run.md))
- Date: 2026-11-26

## Context
ADR 0003 picked GCP Cloud Run. Cloud Run needs a GCP billing account with a working payment method, even inside the free tier. The owner's payment setup failed at GCP sign-up, so the GCP path is blocked. The reasons for Cloud Run still hold: one Docker image, scale to zero, a major cloud on the CV, and deploys from GitHub with no stored key.

## Decision
- Host the same Docker image on **Azure Container Apps** (Consumption plan), with min replicas 0 and max replicas 2, 0.25 vCPU and 0.5 GiB per replica.
- The subscription is **Azure for Students**: sign-up with the university email, no credit card.
- Images go to **GitHub Container Registry** (`ghcr.io/v1d1tt/talent-bridge`, public), not Azure Container Registry, because ACR has no free tier.
- GitHub deploys with **OIDC**: a user-assigned managed identity has a federated credential that trusts tokens from the repo's `azure` GitHub environment, and it has the Contributor role on the `talent-bridge` resource group only. No client secret or key exists.
- **Multiple revision mode.** A push to `main` pins all traffic to its revision. A push to a `slice/*` branch adds a revision with no traffic, under a label (`slice-NN`) that has its own URL.
- `DATABASE_URL` is a GitHub Actions secret, copied into a Container Apps secret on each deploy and read through `secretref:`.
- Key frames (slice 3) move from Cloud Storage to **Azure Blob Storage** with SAS upload URLs.

## Options considered
- **Fix the GCP payment:** no code change, but it depends on a bank-side fix with no known date.
- **Render free web service:** least setup and no card, but it is not a major cloud, and it sleeps after 15 minutes with a wake-up of up to about a minute.
- **Fly.io, Railway, Koyeb:** need a card or have only a short trial.

## Consequences
- The ledger gap "a major cloud" is closed with Azure instead of GCP.
- Student credit and the monthly free grant (180,000 vCPU-seconds, 360,000 GiB-seconds and 2 million requests per subscription) cover this app. When the student offer ends, the subscription must move to pay-as-you-go or the app moves again; the image needs no change.
- Cold start: scale-to-zero on Container Apps plus Neon's own scale-to-zero means the first request after idle can take several seconds. The page already says so.
- Google sign-in (ADR 0004) and Gemini (coach agent) do not depend on GCP billing, so they are not affected.
