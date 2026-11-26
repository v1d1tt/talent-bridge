-- Slice 2: Google sign-in, auth sessions and roles (ADR 0004).
-- The users table is empty before this migration (slice 1 had no sign-up),
-- so the new NOT NULL columns need no default.

-- +goose Up
CREATE TYPE user_role AS ENUM ('athlete', 'coach', 'admin');

ALTER TABLE users
    ADD COLUMN google_sub   text      NOT NULL UNIQUE, -- Google's stable user id ("sub" claim)
    ADD COLUMN email        text      NOT NULL,
    ADD COLUMN display_name text      NOT NULL,
    ADD COLUMN role         user_role,                 -- NULL until the user chooses at first sign-in
    ADD COLUMN banned_at    timestamptz;               -- set by the admin; a banned user cannot sign in

-- One row per signed-in browser. The cookie holds a random 256-bit id; only
-- its SHA-256 hash is stored, so a leaked table cannot be used to sign in.
CREATE TABLE auth_sessions (
    id_hash      bytea       PRIMARY KEY CHECK (octet_length(id_hash) = 32),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_token   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(), -- idle expiry counts from here
    expires_at   timestamptz NOT NULL                -- absolute expiry
);

CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id);

-- +goose Down
DROP TABLE auth_sessions;
ALTER TABLE users
    DROP COLUMN banned_at,
    DROP COLUMN role,
    DROP COLUMN display_name,
    DROP COLUMN email,
    DROP COLUMN google_sub;
DROP TYPE user_role;
