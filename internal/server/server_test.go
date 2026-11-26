package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/server"
)

type upProbe struct{}

func (upProbe) Probe(context.Context) (int64, error) { return 1, nil }

// newTestServer builds the handler over a fake built frontend in a temp dir.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>book</title>"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte("console.log(1)"), 0o644))

	srv := httptest.NewServer(server.New(server.Config{StaticDir: dir, Probe: upProbe{}, Version: "test"}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRoutes(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		name, path   string
		wantCode     int
		wantBody     string
		wantCacheCtl string
	}{
		{"health", "/api/health", 200, `"status":"ok"`, "no-store"},
		{"index", "/", 200, "<title>book</title>", "no-cache"},
		{"hashed asset", "/assets/app-abc123.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"client route falls back to index", "/history/straight_drive", 200, "<title>book</title>", "no-cache"},
		{"unknown api path is a JSON 404", "/api/nope", 404, `"error":"not_found"`, ""},
		{"missing asset is a 404, not index", "/assets/missing.js", 404, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.wantCode {
				t.Fatalf("GET %s = %d, want %d (body %q)", tc.path, res.StatusCode, tc.wantCode, body)
			}
			if !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("GET %s body = %q, want it to contain %q", tc.path, body, tc.wantBody)
			}
			if tc.wantCacheCtl != "" && res.Header.Get("Cache-Control") != tc.wantCacheCtl {
				t.Errorf("GET %s Cache-Control = %q, want %q", tc.path, res.Header.Get("Cache-Control"), tc.wantCacheCtl)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newTestServer(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q, want default-src 'self'", csp)
	}
}

func TestOnlyGetForHealth(t *testing.T) {
	srv := newTestServer(t)
	res, err := http.Post(srv.URL+"/api/health", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/health = %d, want 405", res.StatusCode)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
