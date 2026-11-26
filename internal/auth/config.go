package auth

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// GoogleIssuer is the only issuer the server trusts outside local development.
const GoogleIssuer = "https://accounts.google.com"

// CallbackPath is where Google sends the browser back. The full URL
// (PublicURL + CallbackPath) must be listed in the Google OAuth client.
const CallbackPath = "/auth/google/callback"

// Config is the sign-in configuration. It comes from environment variables
// (LoadConfig), never from the repo.
type Config struct {
	ClientID     string
	ClientSecret string
	// Issuer is Google, except in local development and tests, where it can
	// be the fake provider (internal/auth/fakegoogle) on localhost.
	Issuer string
	// PublicURL is the origin users open, for example
	// https://talent-bridge.<id>.malaysiawest.azurecontainerapps.io.
	PublicURL string
	// AdminEmails get the admin role at sign-in, if Google says the email is
	// verified. Everyone else chooses athlete or coach.
	AdminEmails []string
}

// ErrNotConfigured means the Google client is not set, so sign-in is off.
var ErrNotConfigured = errors.New("sign-in is not configured (GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are not set)")

// LoadConfig reads:
//
//	GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET  the Google OAuth web client
//	PUBLIC_URL                              the app's origin (default http://localhost:8080)
//	ADMIN_EMAILS                            comma-separated admin emails
//	GOOGLE_ISSUER                           local development only: the fake provider's URL
//
// A non-Google issuer is refused unless both the issuer and PUBLIC_URL are on
// this machine (localhost or a loopback IP). So a deployed app can only ever
// trust Google, even if GOOGLE_ISSUER is set there by mistake.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		ClientID:     strings.TrimSpace(getenv("GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET")),
		Issuer:       strings.TrimSuffix(strings.TrimSpace(getenv("GOOGLE_ISSUER")), "/"),
		PublicURL:    strings.TrimSuffix(strings.TrimSpace(getenv("PUBLIC_URL")), "/"),
	}
	for e := range strings.SplitSeq(getenv("ADMIN_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return Config{}, ErrNotConfigured
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:8080"
	}
	pub, err := url.Parse(cfg.PublicURL)
	if err != nil || pub.Host == "" || (pub.Scheme != "https" && pub.Scheme != "http") || pub.Path != "" {
		return Config{}, fmt.Errorf("PUBLIC_URL %q must be an origin such as https://example.com", cfg.PublicURL)
	}
	if pub.Scheme == "http" && !isLoopback(pub) {
		return Config{}, fmt.Errorf("PUBLIC_URL %q must use https (http is only for localhost)", cfg.PublicURL)
	}

	if cfg.Issuer == "" || cfg.Issuer == GoogleIssuer {
		cfg.Issuer = GoogleIssuer
		return cfg, nil
	}
	iss, err := url.Parse(cfg.Issuer)
	if err != nil || !isLoopback(iss) || !isLoopback(pub) {
		return Config{}, fmt.Errorf("GOOGLE_ISSUER %q is refused: a non-Google issuer is allowed only when it and PUBLIC_URL are both on localhost", cfg.Issuer)
	}
	return cfg, nil
}

func isLoopback(u *url.URL) bool {
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) isAdmin(email string, verified bool) bool {
	if !verified {
		return false
	}
	return slices.Contains(c.AdminEmails, strings.ToLower(email))
}
