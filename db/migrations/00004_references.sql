-- Slice 4: shots, pro references and shot gates (PRD section 6).
-- A reference is the pose sequence of one pro player for one shot and one
-- view, stored compressed like an analysis (internal/pose, format TBP1).

-- +goose Up
CREATE TYPE discipline AS ENUM ('batting', 'bowling');

CREATE TABLE shots (
    id         smallint   PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    discipline discipline NOT NULL,
    code       text       NOT NULL UNIQUE CHECK (code ~ '^[a-z][a-z_]{1,39}$'),
    name       text       NOT NULL,
    sort_order smallint   NOT NULL UNIQUE
);

-- The shot list the owner approved on 2026-11-28. A left-handed player
-- uses the same shot: handedness is a tag of its own, and left-handed
-- sequences are mirrored before comparison.
INSERT INTO shots (discipline, code, name, sort_order) VALUES
    ('bowling', 'fast_bowling',     'Fast bowling (fast and medium pace)',            10),
    ('bowling', 'off_spin',         'Off spin (finger spin, also left-arm orthodox)', 20),
    ('bowling', 'leg_spin',         'Leg spin (wrist spin, also left-arm wrist spin)', 30),
    ('batting', 'forward_defence',  'Forward defence',                                 110),
    ('batting', 'backward_defence', 'Backward defence',                                120),
    ('batting', 'straight_drive',   'Straight drive',                                  130),
    ('batting', 'cover_drive',      'Cover drive',                                     140),
    ('batting', 'on_drive',         'On drive',                                        150),
    ('batting', 'lofted_drive',     'Lofted drive',                                    160),
    ('batting', 'cut',              'Cut',                                             170),
    ('batting', 'pull',             'Pull',                                            180),
    ('batting', 'hook',             'Hook',                                            190),
    ('batting', 'flick',            'Flick (also leg glance)',                         200),
    ('batting', 'sweep',            'Sweep',                                           210),
    ('batting', 'reverse_sweep',    'Reverse sweep',                                   220);

CREATE TYPE handedness AS ENUM ('right', 'left');

-- browser: made on the admin page with the same pose code as athletes.
-- legacy_csv: imported once from the 2026 hackathon CSVs (ADR 0008); drawn,
-- but not counted toward a shot gate.
CREATE TYPE reference_origin AS ENUM ('browser', 'legacy_csv');

CREATE TABLE pro_references (
    id            uuid             PRIMARY KEY DEFAULT gen_random_uuid(),
    shot_id       smallint         NOT NULL REFERENCES shots (id),
    view          clip_view        NOT NULL,
    player        text             NOT NULL CHECK (char_length(player) BETWEEN 1 AND 80),
    handedness    handedness       NOT NULL,
    source        text             NOT NULL CHECK (char_length(source) BETWEEN 1 AND 500),
    origin        reference_origin NOT NULL,
    fps           real             NOT NULL CHECK (fps BETWEEN 15 AND 60),
    frame_count   integer          NOT NULL CHECK (frame_count BETWEEN 15 AND 600),
    pose_sequence bytea            NOT NULL CHECK (octet_length(pose_sequence) <= 1000000),
    created_by    uuid             NOT NULL REFERENCES users (id),
    created_at    timestamptz      NOT NULL DEFAULT now()
);

CREATE INDEX pro_references_shot_view_idx ON pro_references (shot_id, view);
-- The import names the old file in `source`, so running it twice adds nothing.
CREATE UNIQUE INDEX pro_references_legacy_source_idx ON pro_references (source)
    WHERE origin = 'legacy_csv';

-- One row per shot + view. reference_count counts browser references only;
-- it is recounted in the same transaction as every new reference. The
-- leave-one-pro-out columns are set by scoring (slices 5-6).
CREATE TABLE shot_gates (
    shot_id         smallint    NOT NULL REFERENCES shots (id) ON DELETE CASCADE,
    view            clip_view   NOT NULL,
    reference_count integer     NOT NULL DEFAULT 0 CHECK (reference_count >= 0),
    lopo_passed     boolean     NOT NULL DEFAULT false,
    lopo_run_at     timestamptz,
    enabled         boolean     NOT NULL DEFAULT false,
    PRIMARY KEY (shot_id, view),
    -- The shot gate rule (GLOSSARY): never on without 3+ references and a passing test.
    CHECK (NOT enabled OR (lopo_passed AND reference_count >= 3))
);

INSERT INTO shot_gates (shot_id, view)
SELECT s.id, v.view FROM shots s CROSS JOIN unnest(enum_range(NULL::clip_view)) AS v (view);

-- +goose Down
DROP TABLE shot_gates;
DROP TABLE pro_references;
DROP TYPE reference_origin;
DROP TYPE handedness;
DROP TABLE shots;
DROP TYPE discipline;
