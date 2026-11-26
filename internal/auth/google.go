package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/v1d1tt/talent-bridge/internal/store"
)

// signinCookie keeps the values of one sign-in attempt between the redirect
// to Google and the callback. It lives 10 minutes.
const signinCookie = "__Host-tb_signin"

const signinMaxAge = 10 * 60

// flow is one sign-in attempt.
type flow struct {
	State    string `json:"s"` // ties the callback to this browser (stops login CSRF)
	Nonce    string `json:"n"` // ties the ID token to this attempt (stops token replay)
	Verifier string `json:"v"` // PKCE: proves the code exchange comes from us
}

// SignInPage is where the browser lands when sign-in fails. The page reads
// the "error" query value.
const SignInPage = "/signin"

func failRedirect(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, SignInPage+"?error="+url.QueryEscape(reason), http.StatusSeeOther)
}

// Start serves GET /auth/google/start: it sends the browser to Google with a
// fresh state, nonce and PKCE challenge.
func (s *Service) Start(w http.ResponseWriter, r *http.Request) {
	if !s.enabled {
		failRedirect(w, r, "unavailable")
		return
	}
	if !s.signInLimit.allow(r) {
		slog.WarnContext(r.Context(), "auth: sign-in rate limit", "ip", clientIP(r))
		failRedirect(w, r, "busy")
		return
	}
	oc, _, err := s.discover()
	if err != nil {
		slog.ErrorContext(r.Context(), "auth: OpenID discovery failed", "issuer", s.cfg.Issuer, "err", err)
		failRedirect(w, r, "unavailable")
		return
	}
	f := flow{State: rand.Text(), Nonce: rand.Text(), Verifier: oauth2.GenerateVerifier()}
	raw, _ := json.Marshal(f)
	setCookie(w, signinCookie, base64.RawURLEncoding.EncodeToString(raw), signinMaxAge)

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, oc.AuthCodeURL(f.State,
		oidc.Nonce(f.Nonce),
		oauth2.S256ChallengeOption(f.Verifier),
		oauth2.SetAuthURLParam("prompt", "select_account"), // let a shared computer pick the account
	), http.StatusFound)
}

func readFlow(r *http.Request) (flow, bool) {
	c, err := r.Cookie(signinCookie)
	if err != nil {
		return flow{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return flow{}, false
	}
	var f flow
	if json.Unmarshal(raw, &f) != nil || f.State == "" || f.Nonce == "" || f.Verifier == "" {
		return flow{}, false
	}
	return f, true
}

func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// idClaims are the ID token claims we use beyond sub.
type idClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// Callback serves GET /auth/google/callback. It checks state, exchanges the
// code (with the PKCE verifier), verifies the ID token and the nonce, then
// starts a new auth session. Every failure goes to the sign-in page with a
// short reason; details go only to the log.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	f, ok := readFlow(r)
	clearCookie(w, signinCookie) // one attempt, one use
	q := r.URL.Query()
	fail := func(reason, msg string, args ...any) {
		slog.WarnContext(r.Context(), "auth: sign-in failed: "+msg, append(args, "reason", reason)...)
		failRedirect(w, r, reason)
	}
	switch {
	case !s.enabled:
		failRedirect(w, r, "unavailable")
		return
	case !s.signInLimit.allow(r):
		fail("busy", "rate limit", "ip", clientIP(r))
		return
	case !ok:
		fail("expired", "no sign-in cookie (older than 10 minutes, or cookies blocked)")
		return
	case q.Get("error") != "":
		fail("cancelled", "the provider returned an error", "error", q.Get("error"))
		return
	case !equal(q.Get("state"), f.State):
		fail("failed", "state does not match")
		return
	}

	oc, verifier, err := s.discover()
	if err != nil {
		fail("unavailable", "OpenID discovery", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)

	tok, err := oc.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		fail("failed", "code exchange", "err", err)
		return
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		fail("failed", "no id_token in the token response")
		return
	}
	idt, err := verifier.Verify(ctx, rawID)
	if err != nil {
		fail("failed", "ID token verification", "err", err)
		return
	}
	if !equal(idt.Nonce, f.Nonce) {
		fail("failed", "nonce does not match")
		return
	}
	var c idClaims
	if err := idt.Claims(&c); err != nil || c.Email == "" {
		fail("failed", "ID token has no email", "err", err)
		return
	}

	u, err := s.q.UpsertGoogleUser(ctx, store.UpsertGoogleUserParams{
		GoogleSub: idt.Subject, Email: c.Email, DisplayName: displayName(c),
	})
	if err != nil {
		fail("unavailable", "save user", "err", err)
		return
	}
	if u.BannedAt.Valid {
		if err := s.q.DeleteUserAuthSessions(ctx, u.ID); err != nil {
			slog.ErrorContext(ctx, "auth: delete banned user's sessions", "err", err)
		}
		fail("banned", "banned user", "user", u.ID.String())
		return
	}
	role, err := s.syncAdmin(ctx, u, c)
	if err != nil {
		fail("unavailable", "set admin role", "err", err)
		return
	}

	// Rotate: drop the session this browser had before, so an id planted
	// before sign-in (session fixation) is worth nothing after it.
	if old, err := r.Cookie(SessionCookie); err == nil {
		if hash, ok := hashSessionID(old.Value); ok {
			_ = s.q.DeleteAuthSession(ctx, hash)
		}
	}
	if err := s.q.DeleteDeadAuthSessions(ctx, int32(s.IdleTimeout.Seconds())); err != nil {
		slog.WarnContext(ctx, "auth: clean up dead sessions", "err", err)
	}
	if err := s.startSession(ctx, w, u.ID); err != nil {
		fail("unavailable", "create session", "err", err)
		return
	}
	slog.InfoContext(ctx, "auth: signed in", "user", u.ID.String(), "role", string(role))

	next := "/"
	if role == RoleNone {
		next = "/welcome" // first sign-in: choose athlete or coach
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// syncAdmin gives the admin role to the configured emails, and takes it away
// from a user who is no longer in the list (they choose again).
func (s *Service) syncAdmin(ctx context.Context, u store.User, c idClaims) (Role, error) {
	role := roleOf(u.Role)
	want := role
	switch {
	case s.cfg.isAdmin(c.Email, c.EmailVerified):
		want = RoleAdmin
	case role == RoleAdmin:
		want = RoleNone
	}
	if want == role {
		return role, nil
	}
	err := s.q.SetUserRole(ctx, store.SetUserRoleParams{
		ID: u.ID, Role: store.NullUserRole{UserRole: store.UserRole(want), Valid: want != RoleNone},
	})
	return want, err
}

// displayName is Google's name, or the part of the email before "@",
// cut to 80 characters.
func displayName(c idClaims) string {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name, _, _ = strings.Cut(c.Email, "@")
	}
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	return name
}
