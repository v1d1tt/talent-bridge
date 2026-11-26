package auth_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/auth"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

const live = "https://talent-bridge.livelyplant-beb9c42a.malaysiawest.azurecontainerapps.io"

func TestLoadConfig(t *testing.T) {
	google := map[string]string{"GOOGLE_CLIENT_ID": "id", "GOOGLE_CLIENT_SECRET": "secret"}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range google {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name       string
		env        map[string]string
		wantIssuer string
		wantErr    string // substring; "" = no error
	}{
		{"no client means sign-in is off", map[string]string{}, "", "not configured"},
		{"secret alone is not enough", map[string]string{"GOOGLE_CLIENT_SECRET": "s"}, "", "not configured"},
		{"production: Google is the issuer", with(map[string]string{"PUBLIC_URL": live}), auth.GoogleIssuer, ""},
		{"local: default public URL", google, auth.GoogleIssuer, ""},
		{"local: the fake provider is allowed", with(map[string]string{"GOOGLE_ISSUER": "http://localhost:9999"}), "http://localhost:9999", ""},
		{"local: loopback IP is allowed", with(map[string]string{"GOOGLE_ISSUER": "http://127.0.0.1:9999", "PUBLIC_URL": "https://127.0.0.1:4443"}), "http://127.0.0.1:9999", ""},
		// The fake provider can never be trusted by a deployed app.
		{"production: a fake issuer is refused", with(map[string]string{"PUBLIC_URL": live, "GOOGLE_ISSUER": "http://localhost:9999"}), "", "refused"},
		{"any non-Google issuer off localhost is refused", with(map[string]string{"GOOGLE_ISSUER": "https://evil.example"}), "", "refused"},
		{"http public URL off localhost is refused", with(map[string]string{"PUBLIC_URL": "http://talent-bridge.example"}), "", "https"},
		{"public URL with a path is refused", with(map[string]string{"PUBLIC_URL": live + "/app"}), "", "origin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := auth.LoadConfig(env(tc.env))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Issuer != tc.wantIssuer {
				t.Errorf("issuer = %q, want %q", cfg.Issuer, tc.wantIssuer)
			}
		})
	}
}

func TestLoadConfigNotConfiguredIsATypedError(t *testing.T) {
	if _, err := auth.LoadConfig(env(nil)); !errors.Is(err, auth.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestLoadConfigAdminEmails(t *testing.T) {
	cfg, err := auth.LoadConfig(env(map[string]string{
		"GOOGLE_CLIENT_ID": "id", "GOOGLE_CLIENT_SECRET": "s", "ADMIN_EMAILS": " Owner@Example.com, ,b@example.com ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"owner@example.com", "b@example.com"}; !slices.Equal(cfg.AdminEmails, want) {
		t.Errorf("admins = %q, want %q", cfg.AdminEmails, want)
	}
}
