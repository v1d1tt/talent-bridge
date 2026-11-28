-- Queries for shots, pro references and shot gates (internal/reference).

-- name: GetShotByCode :one
SELECT * FROM shots WHERE code = $1;

-- name: LockShotGate :one
-- Taken first in the transaction that adds a reference, so two new
-- references of the same shot + view are counted one after the other.
SELECT shot_id FROM shot_gates WHERE shot_id = $1 AND view = $2 FOR UPDATE;

-- name: CreateReference :one
-- A second import of the same old file finds a conflict and returns no row.
INSERT INTO pro_references (shot_id, view, player, handedness, source, origin, fps, frame_count, pose_sequence, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (source) WHERE origin = 'legacy_csv' DO NOTHING
RETURNING id, created_at;

-- name: RecountShotGate :exec
-- Counts browser references only (ADR 0008: old CSV references are not comparable).
UPDATE shot_gates g
SET reference_count = (
    SELECT count(*) FROM pro_references r
    WHERE r.shot_id = g.shot_id AND r.view = g.view AND r.origin = 'browser'
)
WHERE g.shot_id = $1 AND g.view = $2;

-- name: ListReferences :many
-- The reference register: no pose sequences, those load one at a time.
SELECT r.id, s.code AS shot_code, s.name AS shot_name, s.discipline, r.view, r.player,
       r.handedness, r.source, r.origin, r.fps, r.frame_count, r.created_at
FROM pro_references r
JOIN shots s ON s.id = r.shot_id
ORDER BY s.sort_order, r.view, r.created_at;

-- name: GetReference :one
SELECT r.id, s.code AS shot_code, s.name AS shot_name, s.discipline, r.view, r.player,
       r.handedness, r.source, r.origin, r.fps, r.frame_count, r.pose_sequence, r.created_at
FROM pro_references r
JOIN shots s ON s.id = r.shot_id
WHERE r.id = $1;

-- name: ListShotGates :many
-- One row per shot + view, with the old CSV references counted apart.
SELECT s.code AS shot_code, s.name AS shot_name, s.discipline, g.view, g.reference_count,
       (SELECT count(*) FROM pro_references r
        WHERE r.shot_id = g.shot_id AND r.view = g.view AND r.origin = 'legacy_csv') AS legacy_count,
       g.lopo_passed, g.enabled
FROM shot_gates g
JOIN shots s ON s.id = g.shot_id
ORDER BY s.sort_order, g.view;

-- name: GetAdminIDByEmail :one
-- For the one-time import (cmd/import-legacy): the admin it records as created_by.
SELECT id FROM users WHERE lower(email) = lower($1) AND role = 'admin' AND banned_at IS NULL;
