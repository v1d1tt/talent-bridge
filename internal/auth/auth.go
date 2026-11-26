// Package auth is Google sign-in with server-side sessions (ADR 0004).
//
//   - Sign-in: OpenID Connect authorization code flow with PKCE, state and
//     nonce (google.go). The ID token is verified: signature against Google's
//     JWKS, issuer, audience and expiry.
//   - Auth sessions: a random 256-bit id in a __Host- cookie; only its SHA-256
//     hash is stored in Postgres (session.go). Idle and absolute expiry.
//   - CSRF: a random token per auth session, required in the X-CSRF-Token
//     header of every state-changing request.
//   - Roles: one Guard checks every route. An Access zero value lets nobody
//     in, so a route without a rule is denied (access.go).
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/v1d1tt/talent-bridge/internal/store"
)

// Role is a user's role. RoleNone means the user has not chosen yet.
type Role string

const (
	RoleNone    Role = ""
	RoleAthlete Role = "athlete"
	RoleCoach   Role = "coach"
	RoleAdmin   Role = "admin"
)

// Default session limits. A session ends 7 days after its last use, and
// 30 days after sign-in at the latest.
const (
	DefaultIdleTimeout = 7 * 24 * time.Hour
	DefaultLifetime    = 30 * 24 * time.Hour
	// DefaultSignInPerMinute is how many sign-in requests (start + callback)
	// one client IP may send per minute. One sign-in uses 2.
	DefaultSignInPerMinute = 20
)

// Service holds the sign-in configuration and the database queries.
type Service struct {
	enabled bool
	cfg     Config
	q       *store.Queries
	client  *http.Client // for calls to Google, with a timeout

	// IdleTimeout and Lifetime are the session limits (tests shorten them).
	IdleTimeout time.Duration
	Lifetime    time.Duration

	signInLimit limiter // requests per client IP per minute to the sign-in routes

	mu       sync.Mutex // guards the lazy discovery below
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// New returns a sign-in service. It does not call Google yet: discovery runs
// at the first sign-in, so a cold start does not wait for it and a Google
// outage cannot stop the server from starting.
func New(cfg Config, q *store.Queries) *Service {
	return &Service{
		enabled:     true,
		cfg:         cfg,
		q:           q,
		client:      &http.Client{Timeout: 10 * time.Second},
		IdleTimeout: DefaultIdleTimeout,
		Lifetime:    DefaultLifetime,
		signInLimit: limiter{perMinute: DefaultSignInPerMinute},
	}
}

// SetSignInLimit changes the per-IP limit on the sign-in routes; 0 turns it
// off (tests sign in many users from one address).
func (s *Service) SetSignInLimit(perMinute int) { s.signInLimit = limiter{perMinute: perMinute} }

// Disabled returns a service for a server with no database or no Google
// client: sign-in pages say it is unavailable, and every guarded route
// answers 401.
func Disabled() *Service { return &Service{} }

// discover fetches the issuer's OpenID configuration once. A failure is not
// cached, so the next sign-in tries again.
func (s *Service) discover() (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.oauth != nil {
		return s.oauth, s.verifier, nil
	}
	// The provider keeps this context to refresh Google's signing keys later,
	// so it must not be a request context that ends with the request.
	ctx := oidc.ClientContext(context.Background(), s.client)
	p, err := oidc.NewProvider(ctx, s.cfg.Issuer)
	if err != nil {
		return nil, nil, err
	}
	s.oauth = &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.cfg.PublicURL + CallbackPath,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	// Verify checks the signature (JWKS), issuer, audience (our client id)
	// and expiry. The nonce is checked by the caller.
	s.verifier = p.Verifier(&oidc.Config{ClientID: s.cfg.ClientID})
	slog.Info("auth: OpenID configuration loaded", "issuer", s.cfg.Issuer)
	return s.oauth, s.verifier, nil
}
