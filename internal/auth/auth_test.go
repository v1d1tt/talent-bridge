package auth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/auth/authtest"
)

// app is a test handler: the sign-in flow plus one route per access rule.
func app(s *auth.Service) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := "anonymous"
		if sess, ok := auth.SessionFrom(r.Context()); ok {
			role = string(sess.User.Role)
		}
		_, _ = fmt.Fprintf(w, "hello %s", role)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /auth/google/start", s.Guard(auth.Public, http.HandlerFunc(s.Start)))
	mux.Handle("GET "+auth.CallbackPath, s.Guard(auth.Public, http.HandlerFunc(s.Callback)))
	mux.Handle("POST /auth/signout", s.Guard(auth.SignedIn, http.HandlerFunc(s.SignOut)))
	mux.Handle("GET /api/me", s.Guard(auth.Optional, http.HandlerFunc(s.Me)))
	mux.Handle("POST /api/me/role", s.Guard(auth.SignedIn, http.HandlerFunc(s.ChooseRole)))
	for path, a := range map[string]auth.Access{
		"/t/optional":       auth.Optional,
		"/t/signed-in":      auth.SignedIn,
		"/t/athlete":        auth.Roles(auth.RoleAthlete),
		"/t/coach":          auth.Roles(auth.RoleCoach),
		"/t/admin":          auth.Roles(auth.RoleAdmin),
		"/t/coach-or-admin": auth.Roles(auth.RoleCoach, auth.RoleAdmin),
		"/t/no-rule":        {}, // a route someone forgot to give a rule
	} {
		mux.Handle(path, s.Guard(a, ok))
	}
	return mux
}

// TestGuardRoleTable: each access rule x each kind of caller, for a read
// (GET) and a write (POST with the CSRF token).
func TestGuardRoleTable(t *testing.T) {
	e := authtest.New(t, app)
	callers := []auth.Role{"anonymous", auth.RoleNone, auth.RoleAthlete, auth.RoleCoach, auth.RoleAdmin}
	clients := map[auth.Role]*http.Client{"anonymous": e.Client(t)}
	csrf := map[auth.Role]string{}
	for _, r := range callers[1:] {
		c, me := e.As(t, r)
		clients[r], csrf[r] = c, me.CSRFToken
	}

	// want[path] = codes for: anonymous, no role, athlete, coach, admin
	want := map[string][5]int{
		"/t/optional":       {200, 200, 200, 200, 200},
		"/t/signed-in":      {401, 200, 200, 200, 200},
		"/t/athlete":        {401, 403, 200, 403, 403},
		"/t/coach":          {401, 403, 403, 200, 403},
		"/t/admin":          {401, 403, 403, 403, 200},
		"/t/coach-or-admin": {401, 403, 403, 200, 200},
		"/t/no-rule":        {403, 403, 403, 403, 403},
	}
	for path, codes := range want {
		for i, caller := range callers {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				name := fmt.Sprintf("%s %s as %q", method, path, caller)
				t.Run(name, func(t *testing.T) {
					res := e.Do(t, clients[caller], method, path, "", csrf[caller])
					_ = res.Body.Close()
					if res.StatusCode != codes[i] {
						t.Errorf("%s = %d, want %d", name, res.StatusCode, codes[i])
					}
				})
			}
		}
	}
}

func TestCSRFTokenIsRequiredOnStateChangingRequests(t *testing.T) {
	e := authtest.New(t, app)
	c, me := e.As(t, auth.RoleAthlete)
	_, otherMe := e.As(t, auth.RoleAthlete)

	tests := []struct {
		name, method, token string
		want                int
	}{
		{"GET needs no token", http.MethodGet, "", 200},
		{"POST without a token", http.MethodPost, "", 403},
		{"PUT without a token", http.MethodPut, "", 403},
		{"DELETE without a token", http.MethodDelete, "", 403},
		{"POST with a wrong token", http.MethodPost, "not-the-token", 403},
		{"POST with another session's token", http.MethodPost, otherMe.CSRFToken, 403},
		{"POST with the right token", http.MethodPost, me.CSRFToken, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.Do(t, c, tc.method, "/t/signed-in", "", tc.token)
			body, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("%s = %d %s, want %d", tc.method, res.StatusCode, body, tc.want)
			}
			if tc.want == 403 && !strings.Contains(string(body), `"csrf"`) {
				t.Errorf("body = %s, want the csrf error", body)
			}
		})
	}

	t.Run("sign-out without a token is refused and keeps the session", func(t *testing.T) {
		res := e.Do(t, c, http.MethodPost, "/auth/signout", "", "")
		_ = res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("POST /auth/signout = %d, want 403", res.StatusCode)
		}
		e.Me(t, c) // still signed in
	})
}

func TestFirstSignInChoosesARole(t *testing.T) {
	e := authtest.New(t, app)
	c := e.Client(t)
	id := authtest.NewIdentity()

	if next := e.SignIn(t, c, id); next != "/welcome" {
		t.Fatalf("first sign-in goes to %q, want /welcome", next)
	}
	me := e.Me(t, c)
	if me.User.Role != nil || me.User.Email != id.Email || me.User.DisplayName != id.Name {
		t.Fatalf("me = %+v, want no role yet and the Google email and name", me.User)
	}

	choose := func(body string) int {
		res := e.Do(t, c, http.MethodPost, "/api/me/role", body, me.CSRFToken)
		_ = res.Body.Close()
		return res.StatusCode
	}
	if got := choose(`{"role":"admin"}`); got != 422 {
		t.Errorf("choosing admin = %d, want 422 (admin comes from config only)", got)
	}
	if got := choose(`{"role":"athlete","extra":1}`); got != 422 {
		t.Errorf("unknown field = %d, want 422", got)
	}
	if got := choose(`{"role":"coach"}`); got != 200 {
		t.Fatalf("choosing coach = %d, want 200", got)
	}
	if got := choose(`{"role":"athlete"}`); got != 409 {
		t.Errorf("choosing again = %d, want 409", got)
	}
	if r := e.Me(t, c).User.Role; r == nil || *r != "coach" {
		t.Errorf("role = %v, want coach", r)
	}
	// The next sign-in goes straight to the home page.
	if next := e.SignIn(t, e.Client(t), id); next != "/" {
		t.Errorf("second sign-in goes to %q, want /", next)
	}
}

func TestAdminComesFromConfigAndNeedsAVerifiedEmail(t *testing.T) {
	e := authtest.New(t, app)

	unverified := authtest.NewIdentity()
	unverified.Email, unverified.EmailVerified = authtest.AdminEmail, false
	c := e.Client(t)
	if next := e.SignIn(t, c, unverified); next != "/welcome" {
		t.Errorf("unverified admin email goes to %q, want /welcome (no admin role)", next)
	}

	admin := authtest.NewIdentity()
	admin.Email = authtest.AdminEmail
	c = e.Client(t)
	if next := e.SignIn(t, c, admin); next != "/" {
		t.Errorf("admin sign-in goes to %q, want /", next)
	}
	if r := e.Me(t, c).User.Role; r == nil || *r != "admin" {
		t.Errorf("role = %v, want admin", r)
	}
}

func TestBannedUserCannotSignInAndLosesAllSessions(t *testing.T) {
	e := authtest.New(t, app)
	id := authtest.NewIdentity()
	laptop, phone := e.Client(t), e.Client(t)
	e.SignIn(t, laptop, id)
	e.SignIn(t, phone, id)
	me := e.Me(t, laptop)
	if n := sessions(t, e, me.User.ID); n != 2 {
		t.Fatalf("sessions before the ban = %d, want 2", n)
	}

	if err := e.Auth.Ban(context.Background(), me.User.ID); err != nil {
		t.Fatal(err)
	}
	if n := sessions(t, e, me.User.ID); n != 0 {
		t.Errorf("sessions after the ban = %d, want 0", n)
	}
	for name, c := range map[string]*http.Client{"laptop": laptop, "phone": phone} {
		res := e.Do(t, c, http.MethodGet, "/t/signed-in", "", "")
		_ = res.Body.Close()
		if res.StatusCode != 401 {
			t.Errorf("%s: a signed-in route after the ban = %d, want 401", name, res.StatusCode)
		}
	}

	again := e.Client(t)
	if next := e.SignIn(t, again, id); next != "/signin?error=banned" {
		t.Errorf("banned sign-in goes to %q, want /signin?error=banned", next)
	}
	res := e.Do(t, again, http.MethodGet, "/t/signed-in", "", "")
	_ = res.Body.Close()
	if res.StatusCode != 401 {
		t.Errorf("a signed-in route after a banned sign-in = %d, want 401", res.StatusCode)
	}
	if n := sessions(t, e, me.User.ID); n != 0 {
		t.Errorf("sessions after a banned sign-in = %d, want 0", n)
	}
}

// A ban written straight to the users table (without deleting sessions) is
// still caught at the next request, and that request deletes the sessions.
func TestGuardCatchesABanOnTheNextRequest(t *testing.T) {
	e := authtest.New(t, app)
	c, me := e.As(t, auth.RoleAthlete)
	if err := e.DB.Queries.BanUser(context.Background(), uuid(t, me.User.ID)); err != nil {
		t.Fatal(err)
	}
	res := e.Do(t, c, http.MethodGet, "/t/signed-in", "", "")
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 403 || !strings.Contains(string(body), `"banned"`) {
		t.Errorf("signed-in route = %d %s, want 403 banned", res.StatusCode, body)
	}
	if n := sessions(t, e, me.User.ID); n != 0 {
		t.Errorf("sessions = %d, want 0", n)
	}
}

func TestMeIsNullWhenSignedOut(t *testing.T) {
	e := authtest.New(t, app)
	res := e.Do(t, e.Client(t), http.MethodGet, "/api/me", "", "")
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || strings.TrimSpace(string(body)) != `{"user":null}` {
		t.Errorf("GET /api/me signed out = %d %s, want 200 {\"user\":null}", res.StatusCode, body)
	}
}

func TestSignInRotatesTheSessionAndSignOutDeletesIt(t *testing.T) {
	e := authtest.New(t, app)
	c := e.Client(t)
	id := authtest.NewIdentity()
	e.SignIn(t, c, id)
	first := sessionCookie(t, e, c)
	me := e.Me(t, c)

	e.SignIn(t, c, id) // same browser signs in again
	if second := sessionCookie(t, e, c); second == first {
		t.Fatal("the session id did not change at sign-in")
	}
	if n := sessions(t, e, me.User.ID); n != 1 {
		t.Errorf("sessions after a second sign-in in the same browser = %d, want 1 (old one deleted)", n)
	}
	// The old id no longer works.
	if code := withCookie(t, e, first); code != 401 {
		t.Errorf("old session id = %d, want 401", code)
	}

	me = e.Me(t, c)
	res := e.Do(t, c, http.MethodPost, "/auth/signout", "", me.CSRFToken)
	_ = res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatalf("sign-out = %d, want 204", res.StatusCode)
	}
	if n := sessions(t, e, me.User.ID); n != 0 {
		t.Errorf("sessions after sign-out = %d, want 0", n)
	}
	res = e.Do(t, c, http.MethodGet, "/t/signed-in", "", "")
	_ = res.Body.Close()
	if res.StatusCode != 401 {
		t.Errorf("a signed-in route after sign-out = %d, want 401", res.StatusCode)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	e := authtest.New(t, app)
	c := e.Client(t)
	noFollow := *c
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// Run the flow by hand to see the callback's Set-Cookie header.
	res, err := noFollow.Get(e.App.URL + "/auth/google/start")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	checkCookie(t, res, "__Host-tb_signin")
	loc, _ := res.Location()
	if q := loc.Query(); q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Errorf("auth URL %s lacks PKCE S256, nonce or state", loc)
	}
	id := authtest.NewIdentity()
	form := loc.Query()
	form.Set("sub", id.Sub)
	form.Set("email", id.Email)
	loc.RawQuery = ""
	res, err = noFollow.PostForm(loc.String(), form)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	cb, _ := res.Location()
	res, err = noFollow.Get(cb.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	checkCookie(t, res, auth.SessionCookie)
}

func checkCookie(t *testing.T, res *http.Response, name string) {
	t.Helper()
	for _, ck := range res.Cookies() {
		if ck.Name != name {
			continue
		}
		if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.Domain != "" {
			t.Errorf("%s = %+v, want HttpOnly, Secure, SameSite=Lax, Path=/, no Domain", name, ck)
		}
		return
	}
	t.Errorf("no %s cookie set", name)
}

func TestIdleAndAbsoluteExpiry(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		e := authtest.New(t, app)
		e.Auth.IdleTimeout = time.Second
		c := e.Client(t)
		e.SignIn(t, c, authtest.NewIdentity())
		e.Me(t, c) // used: the idle clock restarts
		time.Sleep(1500 * time.Millisecond)
		res := e.Do(t, c, http.MethodGet, "/t/signed-in", "", "")
		_ = res.Body.Close()
		if res.StatusCode != 401 {
			t.Errorf("after the idle limit = %d, want 401", res.StatusCode)
		}
	})
	t.Run("absolute", func(t *testing.T) {
		e := authtest.New(t, app)
		e.Auth.Lifetime = 2 * time.Second
		c := e.Client(t)
		e.SignIn(t, c, authtest.NewIdentity())
		id := sessionCookie(t, e, c)
		for range 3 { // keep it busy: activity does not extend the absolute limit
			time.Sleep(800 * time.Millisecond)
		}
		// Send the old id by hand: the browser would drop the cookie itself
		// (Max-Age), but the server must refuse it too.
		if code := withCookie(t, e, id); code != 401 {
			t.Errorf("after the absolute limit = %d, want 401", code)
		}
	})
}

func TestCallbackRefusesForgedOrStaleRequests(t *testing.T) {
	e := authtest.New(t, app)
	noFollow := func(c *http.Client) *http.Client {
		n := *c
		n.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return &n
	}
	// callbackURL runs the flow up to the provider's redirect back to the app.
	callbackURL := func(c *http.Client) *url.URL {
		res, err := noFollow(c).Get(e.App.URL + "/auth/google/start")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		loc, _ := res.Location()
		id := authtest.NewIdentity()
		form := loc.Query()
		form.Set("sub", id.Sub)
		form.Set("email", id.Email)
		loc.RawQuery = ""
		res, err = noFollow(c).PostForm(loc.String(), form)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		cb, _ := res.Location()
		return cb
	}
	land := func(c *http.Client, u string) string {
		res, err := noFollow(c).Get(u)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		loc, _ := res.Location()
		return loc.RequestURI()
	}

	t.Run("state from another attempt (login CSRF)", func(t *testing.T) {
		c := e.Client(t)
		cb := callbackURL(c)
		q := cb.Query()
		q.Set("state", "attacker-state")
		cb.RawQuery = q.Encode()
		if got := land(c, cb.String()); got != "/signin?error=failed" {
			t.Errorf("landed on %q, want /signin?error=failed", got)
		}
	})
	t.Run("callback in a browser that did not start the sign-in", func(t *testing.T) {
		cb := callbackURL(e.Client(t))
		if got := land(e.Client(t), cb.String()); got != "/signin?error=expired" {
			t.Errorf("landed on %q, want /signin?error=expired", got)
		}
	})
	t.Run("user cancelled at Google", func(t *testing.T) {
		c := e.Client(t)
		cb := callbackURL(c)
		q := cb.Query()
		q.Del("code")
		q.Set("error", "access_denied")
		cb.RawQuery = q.Encode()
		if got := land(c, cb.String()); got != "/signin?error=cancelled" {
			t.Errorf("landed on %q, want /signin?error=cancelled", got)
		}
	})
	// tamper rewrites one value of the sign-in cookie in the browser.
	tamper := func(c *http.Client, field string) {
		u, _ := url.Parse(e.App.URL)
		for _, ck := range c.Jar.Cookies(u) {
			if ck.Name != "__Host-tb_signin" {
				continue
			}
			raw, _ := base64.RawURLEncoding.DecodeString(ck.Value)
			var f map[string]string
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			f[field] = "tampered-value-that-is-long-enough-for-pkce-0123456789"
			raw, _ = json.Marshal(f)
			c.Jar.SetCookies(u, []*http.Cookie{{Name: ck.Name, Value: base64.RawURLEncoding.EncodeToString(raw), Path: "/", Secure: true, HttpOnly: true}})
			return
		}
		t.Fatal("no sign-in cookie")
	}
	t.Run("ID token with another nonce (replay)", func(t *testing.T) {
		c := e.Client(t)
		cb := callbackURL(c)
		tamper(c, "n")
		if got := land(c, cb.String()); got != "/signin?error=failed" {
			t.Errorf("landed on %q, want /signin?error=failed", got)
		}
	})
	t.Run("wrong PKCE verifier (stolen code)", func(t *testing.T) {
		c := e.Client(t)
		cb := callbackURL(c)
		tamper(c, "v")
		if got := land(c, cb.String()); got != "/signin?error=failed" {
			t.Errorf("landed on %q, want /signin?error=failed", got)
		}
	})
	t.Run("a code is used once", func(t *testing.T) {
		c := e.Client(t)
		cb := callbackURL(c)
		if got := land(c, cb.String()); got != "/welcome" {
			t.Fatalf("first use landed on %q, want /welcome", got)
		}
		// Replaying needs the sign-in cookie again: start a new attempt to get
		// one, then send the old code with the new state.
		fresh := callbackURL(c)
		q := cb.Query()
		q.Set("state", fresh.Query().Get("state"))
		cb.RawQuery = q.Encode()
		if got := land(c, cb.String()); got != "/signin?error=failed" {
			t.Errorf("replayed code landed on %q, want /signin?error=failed", got)
		}
	})
}

func TestDisabledServiceRefusesEverything(t *testing.T) {
	s := auth.Disabled()
	h := app(s)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/auth/google/start", 303},
		{"GET", "/api/me", 200},
		{"GET", "/t/signed-in", 401},
	} {
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

// --- helpers

func sessions(t *testing.T, e *authtest.Env, userID string) int64 {
	t.Helper()
	n, err := e.DB.Queries.CountUserAuthSessions(context.Background(), uuid(t, userID))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func uuid(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

func sessionCookie(t *testing.T, e *authtest.Env, c *http.Client) string {
	t.Helper()
	u, _ := url.Parse(e.App.URL)
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == auth.SessionCookie {
			return ck.Value
		}
	}
	t.Fatal("no session cookie in the jar")
	return ""
}

// withCookie calls a signed-in route with only the given session id.
func withCookie(t *testing.T, e *authtest.Env, id string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.App.URL+"/t/signed-in", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: id})
	res, err := e.App.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func TestSignInRoutesAreRateLimitedPerClient(t *testing.T) {
	e := authtest.New(t, app)
	e.Auth.SetSignInLimit(3)
	start := func(forwardedFor string) string {
		req, _ := http.NewRequest(http.MethodGet, e.App.URL+"/auth/google/start", nil)
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		c := e.Client(t)
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		loc, _ := res.Location()
		return loc.RequestURI()
	}
	for i := range 3 {
		if got := start("203.0.113.7"); strings.HasPrefix(got, "/signin") {
			t.Fatalf("request %d was refused: %s", i+1, got)
		}
	}
	if got := start("203.0.113.7"); got != "/signin?error=busy" {
		t.Errorf("4th request in a minute went to %q, want /signin?error=busy", got)
	}
	// Only the last X-Forwarded-For entry (added by the ingress) counts, so a
	// client cannot dodge the limit by forging the first one.
	if got := start("198.51.100.1, 203.0.113.7"); got != "/signin?error=busy" {
		t.Errorf("forged first entry went to %q, want /signin?error=busy", got)
	}
	if got := start("203.0.113.8"); strings.HasPrefix(got, "/signin") {
		t.Errorf("another client was refused: %s", got)
	}
}

func TestAdminRoleEndsWhenTheEmailLeavesTheConfig(t *testing.T) {
	e := authtest.New(t, app)
	c, _ := e.As(t, auth.RoleAdmin)
	id := sessionCookie(t, e, c)
	if code := get(t, e.App, "/t/admin", id); code != 200 {
		t.Fatalf("admin route = %d, want 200", code)
	}

	// The owner removes the email from ADMIN_EMAILS and the server restarts:
	// a new service, same database, same session.
	restarted := httptest.NewTLSServer(app(auth.New(auth.Config{
		ClientID: "test-client", ClientSecret: "test-secret", Issuer: e.Google.URL, PublicURL: e.App.URL,
	}, e.DB.Queries)))
	defer restarted.Close()
	if code := get(t, restarted, "/t/admin", id); code != 403 {
		t.Errorf("admin route after removal = %d, want 403 (no waiting for the next sign-in)", code)
	}
	if code := get(t, restarted, "/t/signed-in", id); code != 200 {
		t.Errorf("signed-in route after removal = %d, want 200 (still signed in, without a role)", code)
	}
}

// get calls path on srv with only the given session id.
func get(t *testing.T, srv *httptest.Server, path, id string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: id})
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}
