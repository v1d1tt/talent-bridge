-- Slice 3: analyses (sessions) and their key frames (PRD section 6).
-- The pose sequence is stored compressed (internal/pose, format TBP1).
-- Score columns exist now and stay NULL until scoring (slice 5). The columns
-- that point to shots and pro references (shot_id, detected_shot_id,
-- detected_confidence, closest_reference_id) come with those tables.

-- +goose Up
CREATE TYPE clip_view AS ENUM ('side_on', 'front_on');

CREATE TABLE analyses (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    fps           real        NOT NULL CHECK (fps BETWEEN 15 AND 60),
    frame_count   integer     NOT NULL CHECK (frame_count BETWEEN 15 AND 600),
    pose_sequence bytea       NOT NULL CHECK (octet_length(pose_sequence) <= 1000000),
    view          clip_view,             -- slice 5 detects it
    metrics       jsonb,                 -- slice 5
    score         smallint    CHECK (score BETWEEN 0 AND 100), -- slice 5
    faults        jsonb,                 -- slice 5
    coaching      jsonb,                 -- slice 7
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX analyses_user_id_created_at_idx ON analyses (user_id, created_at);

-- A key frame is a still JPEG in Azure Blob Storage. The browser uploads it
-- straight to Blob Storage with a short-lived SAS URL; the server then
-- checks it (size, JPEG bytes) and marks it uploaded or rejected. When the
-- upload URL has expired, a sweep settles every key frame that is not
-- uploaded one last time (settled_at), deleting anything left behind.
CREATE TYPE key_frame_state AS ENUM ('pending', 'uploaded', 'rejected');

CREATE TABLE key_frames (
    id          uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id uuid            NOT NULL REFERENCES analyses (id) ON DELETE CASCADE,
    frame_index integer         NOT NULL CHECK (frame_index BETWEEN 0 AND 599),
    phase       text,                        -- slice 5 names the phase
    object_path text            NOT NULL UNIQUE,
    state       key_frame_state NOT NULL DEFAULT 'pending',
    byte_size   integer,
    created_at  timestamptz     NOT NULL DEFAULT now(),
    settled_at  timestamptz,                 -- set by the sweep after the upload window
    UNIQUE (analysis_id, frame_index)
);

CREATE INDEX key_frames_unsettled_idx ON key_frames (created_at)
    WHERE settled_at IS NULL AND state <> 'uploaded';

-- +goose Down
DROP TABLE key_frames;
DROP TYPE key_frame_state;
DROP TABLE analyses;
DROP TYPE clip_view;
