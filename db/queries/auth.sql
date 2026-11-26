-- Queries for Google sign-in and auth sessions (internal/auth).

-- name: UpsertGoogleUser :one
-- Called after Google verified the user. Email and name follow Google's
-- latest values; role and ban stay as they are.
INSERT INTO users (google_sub, email, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (google_sub) DO UPDATE
    SET email = EXCLUDED.email, display_name = EXCLUDED.display_name
RETURNING *;

-- name: SetUserRole :exec
-- Used only for the admin role, which comes from config.
UPDATE users SET role = $2 WHERE id = $1;

-- name: ChooseRole :one
-- The first-sign-in choice. It changes nothing once a role is set.
UPDATE users SET role = $2 WHERE id = $1 AND role IS NULL
RETURNING role;

-- name: BanUser :exec
UPDATE users SET banned_at = now() WHERE id = $1 AND banned_at IS NULL;

-- name: CreateAuthSession :exec
INSERT INTO auth_sessions (id_hash, user_id, csrf_token, expires_at)
VALUES ($1, $2, $3, now() + make_interval(secs => sqlc.arg(lifetime_seconds)::int));

-- name: TouchAuthSession :one
-- Looks up a live session and marks it as used now. A session is live while
-- it is before its absolute expiry and was used within the idle limit.
UPDATE auth_sessions AS s
SET last_seen_at = now()
FROM users AS u
WHERE s.id_hash = $1
  AND u.id = s.user_id
  AND s.expires_at > now()
  AND s.last_seen_at > now() - make_interval(secs => sqlc.arg(idle_seconds)::int)
RETURNING s.user_id, s.csrf_token, u.email, u.display_name, u.role, u.banned_at;

-- name: DeleteAuthSession :exec
DELETE FROM auth_sessions WHERE id_hash = $1;

-- name: DeleteUserAuthSessions :exec
DELETE FROM auth_sessions WHERE user_id = $1;

-- name: DeleteDeadAuthSessions :exec
-- Housekeeping at each sign-in: rows that can never be used again.
DELETE FROM auth_sessions
WHERE expires_at <= now()
   OR last_seen_at <= now() - make_interval(secs => sqlc.arg(idle_seconds)::int);

-- name: CountUserAuthSessions :one
SELECT count(*) FROM auth_sessions WHERE user_id = $1;
