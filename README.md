# Talent-Bridge

Talent-Bridge started as my first hackathon build (Feb 2026), and the team did not finish it. I am now rebuilding it as v2.

**v2 goal:** an athlete records a cricket clip, and the app scores the technique against pro players for that exact shot and camera view. The app shows the main faults and gives a drill plan. Athletes can choose to be found by coaches.

**Status:** planning is done, and the build has not started. The plan is in these files:

- [docs/PRD.md](docs/PRD.md): requirements, stack, data model, API and slices
- [GLOSSARY.md](GLOSSARY.md): domain words
- [docs/adr/](docs/adr/): architecture decisions
- Slices: GitHub issues #1-#11

**Planned stack:** Go 1.27 (sqlc, goose, Google ADK for Go v2) · Postgres on Neon · SolidJS 2 + Vite · MediaPipe Pose Landmarker in the browser · Docker on GCP Cloud Run.

The old hackathon code (Python, OpenCV, an LSTM, Gemini) is still in the repo. Slice 1 removes it, and it stays in the git history.
