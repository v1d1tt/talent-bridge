# 0007. A fake Google on localhost for tests and local sign-in

- Status: accepted
- Date: 2026-11-27

## Context
Slice 2 adds Google sign-in (ADR 0004). Tests, CI and local development must sign in without a real Google account: CI has no browser that can type a Google password, and a contributor should not need the owner's OAuth client. A common shortcut is a "dev login" route that signs anyone in with no checks. Such a route is one wrong setting away from being live in production, and it skips exactly the code that must be tested (PKCE, state, nonce, ID token checks).

## Decision
- A small OpenID Connect provider, `internal/auth/fakegoogle`, plays Google on localhost. It serves discovery, a form instead of Google's account chooser, a token endpoint that checks the client secret, the redirect URI and the PKCE verifier, and RS256 ID tokens with a JWKS endpoint. `cmd/fake-google` runs it for local development and Playwright.
- The server has **no** dev-only sign-in code. It runs the same flow against the fake as against Google; only `GOOGLE_ISSUER` differs.
- `auth.LoadConfig` refuses any issuer other than `https://accounts.google.com` unless both the issuer and `PUBLIC_URL` are on localhost or a loopback IP. The server then stops at start with an error. So a deployed app can only trust Google, even if `GOOGLE_ISSUER` is set there by mistake. Table tests in `internal/auth/config_test.go` check this, including "production: a fake issuer is refused".
- The fake is not in the Docker image: the Dockerfile builds only `./cmd/server`, and the server does not import the fake.

## Options considered
- **Dev-only login route, compiled out with a build tag:** less code, but CI would then test a different binary from production, and the real flow would have no end-to-end test.
- **Mock the OIDC library in unit tests:** fast, but it tests our mock, not the token checks.
- **A real test Google account in CI:** Google blocks automated password sign-in; and it would put a credential in CI.

## Consequences
- The whole flow (state, nonce, PKCE, signature, issuer, audience, expiry) runs in Go tests and in Playwright on every push.
- The fake must stay close to Google's behaviour. Differences it does not model: Google's consent screen, `accounts.google.com` without `https://` as an issuer (go-oidc accepts that for Google only), and key rotation.
