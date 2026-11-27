-- Queries for analyses and key frames (internal/analysis).

-- name: CreateAnalysis :one
INSERT INTO analyses (user_id, fps, frame_count, pose_sequence)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at;

-- name: CreateKeyFrame :one
INSERT INTO key_frames (analysis_id, frame_index, object_path)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CountAnalysesSince :one
-- For the per-athlete daily limit.
SELECT count(*) FROM analyses WHERE user_id = $1 AND created_at > $2;

-- name: GetOwnAnalysis :one
-- Owner only: another user's id finds no row, so the API answers 404.
SELECT id, fps, frame_count, pose_sequence, created_at
FROM analyses
WHERE id = $1 AND user_id = $2;

-- name: ListKeyFrames :many
SELECT * FROM key_frames WHERE analysis_id = $1 ORDER BY frame_index;

-- name: SetKeyFrameState :exec
-- Only a pending key frame changes, so a second check cannot undo a result.
UPDATE key_frames SET state = $2, byte_size = $3
WHERE id = $1 AND state = 'pending';

-- name: ListUnsettledKeyFrames :many
-- Key frames whose upload window closed before $1 and that are not final yet.
SELECT * FROM key_frames
WHERE settled_at IS NULL AND state <> 'uploaded' AND created_at < $1
ORDER BY created_at
LIMIT 200;

-- name: SettleKeyFrame :exec
UPDATE key_frames SET state = $2, byte_size = $3, settled_at = now()
WHERE id = $1 AND settled_at IS NULL;
