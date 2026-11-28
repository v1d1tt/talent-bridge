package server_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/auth/authtest"
	"github.com/v1d1tt/talent-bridge/internal/server"
)

// callers are the kinds of request the table checks, in column order.
var callers = []auth.Role{"anonymous", auth.RoleNone, auth.RoleAthlete, auth.RoleCoach, auth.RoleAdmin}

// routeTable is the expected answer of every route for each caller:
// anonymous, signed in without a role, athlete, coach, admin. Signed-in
// callers send their CSRF token, so the table checks access, not CSRF.
// Adding a route to server.Routes without a row here fails the test.
var routeTable = map[string]struct {
	body  string
	codes [5]int
}{
	"GET /api/health":           {"", [5]int{200, 200, 200, 200, 200}},
	"GET /auth/google/start":    {"", [5]int{302, 302, 302, 302, 302}},
	"GET /auth/google/callback": {"", [5]int{303, 303, 303, 303, 303}}, // no attempt in progress -> /signin?error=expired
	"POST /auth/signout":        {"", [5]int{401, 204, 204, 204, 204}},
	"GET /api/me":               {"", [5]int{200, 200, 200, 200, 200}}, // signed out: {"user": null}
	"POST /api/me/role":         {`{"role":"athlete"}`, [5]int{401, 200, 409, 409, 409}},
	// Athletes only. This server has no analysis service (no database), so an
	// athlete gets 503; internal/analysis tests the owner rule with a database.
	"POST /api/analyses":                       {"", [5]int{401, 403, 503, 403, 403}},
	"GET /api/analyses/{id}":                   {"", [5]int{401, 403, 503, 403, 403}},
	"POST /api/analyses/{id}/key-frames/check": {"", [5]int{401, 403, 503, 403, 403}},
	// Admin only. No reference service here (no database), so the admin gets
	// 503; internal/reference tests the routes with a database.
	"POST /api/admin/references":     {"", [5]int{401, 403, 403, 403, 503}},
	"GET /api/admin/references":      {"", [5]int{401, 403, 403, 403, 503}},
	"GET /api/admin/references/{id}": {"", [5]int{401, 403, 403, 403, 503}},
	"GET /api/admin/shot-gates":      {"", [5]int{401, 403, 403, 403, 503}},
}

func TestRouteRoleTable(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644))
	e := authtest.New(t, func(s *auth.Service) http.Handler {
		return server.New(server.Config{StaticDir: dir, Probe: upProbe{}, Version: "test", Auth: s})
	})

	routes := server.Routes(server.Config{Auth: e.Auth})
	for _, rt := range routes {
		if _, ok := routeTable[rt.Pattern]; !ok {
			t.Errorf("route %q (access %s) has no row in routeTable", rt.Pattern, rt.Access)
		}
	}
	if len(routes) != len(routeTable) {
		t.Errorf("routeTable has %d rows, server.Routes has %d routes", len(routeTable), len(routes))
	}

	for _, rt := range routes {
		row := routeTable[rt.Pattern]
		var method, path string
		_, _ = fmt.Sscanf(rt.Pattern, "%s %s", &method, &path)
		for i, caller := range callers {
			name := fmt.Sprintf("%s as %q", rt.Pattern, caller)
			t.Run(name, func(t *testing.T) {
				// A fresh caller per cell: sign-out and role choice change state.
				c, csrf := e.Client(t), ""
				if caller != "anonymous" {
					var me authtest.Me
					c, me = e.As(t, caller)
					csrf = me.CSRFToken
				}
				res := e.Do(t, c, method, path, row.body, csrf)
				_ = res.Body.Close()
				if res.StatusCode != row.codes[i] {
					t.Errorf("%s = %d, want %d", name, res.StatusCode, row.codes[i])
				}
			})
		}
	}
}

func TestEveryRouteHasAnAccessRule(t *testing.T) {
	for _, rt := range server.Routes(server.Config{Auth: auth.Disabled()}) {
		if rt.Access.String() == "nobody" {
			t.Errorf("route %q has no access rule (the zero rule denies everyone)", rt.Pattern)
		}
	}
}

func TestWrongMethodOnAKnownPathIs405(t *testing.T) {
	srv := newTestServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/me"},
		{http.MethodGet, "/auth/signout"},
		{http.MethodGet, "/api/me/role"},
	} {
		req, _ := http.NewRequest(tc.method, srv.URL+tc.path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", tc.method, tc.path, res.StatusCode)
		}
	}
}
