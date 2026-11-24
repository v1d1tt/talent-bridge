# 0001. Pose extraction runs in the browser

- Status: accepted
- Date: 2026-11-25

## Context
Every analysis starts with a clip. Clips are large, personal, and often show minors. The old build had no pose step at all: the pose CSVs were made outside the repo.

## Decision
Run MediaPipe Pose Landmarker for Web (`@mediapipe/tasks-vision` 1.2.3, `VIDEO` running mode, `detectForVideo`) in the browser. The browser sends only the pose sequence (landmarks JSON) and 2-3 key frame JPEGs. The server never receives the clip. The admin page uses the same code path to create references, so athletes and references are measured the same way.

## Options considered
- Python server with MediaPipe: easier to test, but needs a heavier host, uploads full clips, and stores personal video for a short time.
- Browser (picked): no GPU server, free hosting, the video stays on the device.

## Consequences
- Slow devices take longer to process a clip. Use the `lite` or `full` model, picked by a benchmark in the pose slice.
- The server must not trust the landmarks to be honest. Scores are computed on the server, and the API rejects sequences with the wrong shape, impossible values or too few frames.
- The model file and the WASM files must be served under the Apache 2.0 terms (keep their notices).
