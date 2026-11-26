// Package authtest builds what the auth tests need: a migrated test
// database, a fake Google on localhost, the app under TLS (the __Host-
// cookie is Secure), and signed-in clients for each role.
package authtest

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/auth/fakegoogle"
	"github.com/v1d1tt/talent-bridge/internal/database"
)

const (
	clientID     = "test-client"
	clientSecret = "test-secret"
	// AdminEmail is in the test config's admin list.
	AdminEmail = "owner@example.com"
)

// Env is one test's world.
type Env struct {
	DB     *database.DB
	Auth   *auth.Service
	App    *httptest.Server // the app under test, over TLS
	Google *httptest.Server // the fake provider
}

// New starts the fake provider and the app. handler builds the app's root
// handler from the sign-in service (for example server.New). Tests skip
// when TEST_DATABASE_URL is not set.
func New(t *testing.T, handler func(*auth.Service) http.Handler) *Env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err := db.Probe(ctx); err != nil { // runs the migrations
		t.Fatal(err)
	}

	google := httptest.NewUnstartedServer(nil)
	issuer := "http://" + google.Listener.Addr().String()
	fake, err := fakegoogle.New(issuer, clientID, clientSecret)
	if err != nil {
		t.Fatal(err)
	}
	google.Config.Handler = fake.Handler()
	google.Start()
	t.Cleanup(google.Close)

	// The app's URL is known only once it listens, and the auth config needs
	// it for the redirect URI, so the handler is set after the start.
	var root http.Handler
	app := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { root.ServeHTTP(w, r) }))
	t.Cleanup(app.Close)
	svc := auth.New(auth.Config{
		ClientID: clientID, ClientSecret: clientSecret, Issuer: issuer,
		PublicURL: app.URL, AdminEmails: []string{AdminEmail},
	}, db.Queries)
	svc.SetSignInLimit(0) // every test signs in many users from 127.0.0.1
	root = handler(svc)
	return &Env{DB: db, Auth: svc, App: app, Google: google}
}

// Client is a fresh browser: its own cookie jar, trusting the app's TLS.
func (e *Env) Client(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := *e.App.Client() // a copy: Client() returns one shared client
	c.Jar = jar
	return &c
}

// NewIdentity is a Google account no other test uses.
func NewIdentity() fakegoogle.Identity {
	id := rand.Text()
	return fakegoogle.Identity{Sub: "sub-" + id, Email: strings.ToLower(id) + "@example.com", EmailVerified: true, Name: "Player " + id[:4]}
}

// Me is the body of GET /api/me.
type Me struct {
	User struct {
		ID          string  `json:"id"`
		Email       string  `json:"email"`
		DisplayName string  `json:"display_name"`
		Role        *string `json:"role"`
	} `json:"user"`
	CSRFToken string `json:"csrf_token"`
}

// SignIn signs a client in as identity and returns where the app sent it.
func (e *Env) SignIn(t *testing.T, c *http.Client, id fakegoogle.Identity) string {
	t.Helper()
	next, err := fakegoogle.SignIn(c, e.App.URL, id)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// As returns a signed-in client with the role: "" (not chosen yet),
// "athlete", "coach" or "admin".
func (e *Env) As(t *testing.T, role auth.Role) (*http.Client, Me) {
	t.Helper()
	c := e.Client(t)
	id := NewIdentity()
	if role == auth.RoleAdmin {
		id.Sub, id.Email = "sub-admin-"+rand.Text(), AdminEmail
	}
	e.SignIn(t, c, id)
	me := e.Me(t, c)
	if role == auth.RoleAthlete || role == auth.RoleCoach {
		res := e.Do(t, c, http.MethodPost, "/api/me/role", `{"role":"`+string(role)+`"}`, me.CSRFToken)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("choose role %s = %d", role, res.StatusCode)
		}
		me = e.Me(t, c)
	}
	return c, me
}

// Me calls GET /api/me and fails the test unless it answers 200.
func (e *Env) Me(t *testing.T, c *http.Client) Me {
	t.Helper()
	res := e.Do(t, c, http.MethodGet, "/api/me", "", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me = %d, want 200", res.StatusCode)
	}
	var me Me
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	return me
}

// Do sends one request. csrf goes in the X-CSRF-Token header when not empty.
func (e *Env) Do(t *testing.T, c *http.Client, method, path, body, csrf string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.App.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set(auth.CSRFHeader, csrf)
	}
	noFollow := *c
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
