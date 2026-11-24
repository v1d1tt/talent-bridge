# 0004. Own server-side sessions with Google sign-in

- Status: accepted
- Date: 2026-11-25

## Context
There are three roles (athlete, coach, admin), and a public scouting list that may show minors. Auth must be correct and explainable in an interview.

## Decision
- Sign-in with Google OAuth 2.0 / OpenID Connect (authorization code flow with PKCE and `state`; verify the ID token).
- Server-side sessions in Postgres (`auth_sessions`): random 256-bit id, stored hashed, sent as `__Host-` cookie with `HttpOnly`, `Secure`, `SameSite=Lax`. Idle and absolute expiry. Rotate the id on sign-in.
- CSRF token per auth session, required on every state-changing request.
- Roles in the `users` table. One middleware checks the role on every route. Deny by default.
- Banned users cannot sign in, and their auth sessions are deleted.

## Options considered
- Hosted auth (Clerk, Auth0): fastest, but hides the parts that matter for learning.
- Email + password: more to build (hashing, reset emails) for no extra value here.

## Consequences
- Users need a Google account (minimum age 13).
- The age band comes from the user's own entry, not from Google, so it can be false. The guardian-consent rule (ADR 0005) depends on it.
