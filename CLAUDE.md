# Talent-Bridge

Cricket technique analysis and scouting app. This repo is project 2 of the portfolio upgrade. Load the `project-upgrade` skill for any work here.

## Project rules

- **Git:** commit only under the user's own git identity. No `Co-Authored-By: Claude` line and no mention of Claude in commit messages, PR titles or PR text.
- **Branch + PR per slice:** each slice works on its own branch (`slice/<NN>-<short-name>`) cut from `origin/main`. At the end of the slice, after its tests pass: one commit, push the branch, open a PR whose body says `Closes #<NN>` (issue number = slice number). Never push mid-slice, never push to `main` directly.
- **Merging (merge rights granted by the owner on 2026-11-25):** the agent turns on auto-merge for each PR with `gh pr merge <n> --auto --merge --delete-branch`. The PR merges itself with a merge commit when the required CI check is green. Slice 1 adds the CI workflow and makes it a required check on `main`. Merged branches are deleted: GitHub deletes the remote branch, and the agent deletes the local branch and updates local `main`.
- **README:** update `README.md` in the same commit as each slice, so it always matches the code.
- **Docs first:** look up every library and platform API in Context7 before writing code for it. Solid is version 2 (`@solidjs/web`, `@solidjs/vite-plugin`); do not use Solid 1 APIs.
- **Learning notes:** each slice adds `docs/learning/<NN>.md`.
- **Secrets:** never commit `.env` files, Google OAuth secrets, Gemini keys or GCP credentials. CI deploys with Workload Identity Federation, not JSON keys.

## Agent skills

### Issue tracker

The PRD is `docs/PRD.md`. Slice tickets are GitHub Issues #1-#11 on `v1d1tt/talent-bridge` (use the `gh` CLI). Each issue has a "Blocked by" line.

### Domain docs

Single context: `GLOSSARY.md` and `docs/adr/` at the repo root. Add a new domain word to the glossary before using it in code.
