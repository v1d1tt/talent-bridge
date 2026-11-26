-- Stub of the users table. Slice 2 (Google sign-in) adds google_sub, email,
-- display_name and role in a new migration.

-- +goose Up
CREATE TABLE users (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
