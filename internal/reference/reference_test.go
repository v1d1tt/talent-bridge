package reference_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/auth/authtest"
	"github.com/v1d1tt/talent-bridge/internal/pose"
	"github.com/v1d1tt/talent-bridge/internal/reference"
	"github.com/v1d1tt/talent-bridge/internal/server"
	"github.com/v1d1tt/talent-bridge/internal/store"
)

type upProbe struct{}

func (upProbe) Probe(context.Context) (int64, error) { return 1, nil }

// world is the app with a database and the fake Google.
type world struct {
	*authtest.Env
	svc *reference.Service
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &world{}
	var once sync.Once
	var app http.Handler
	w.Env = authtest.New(t, func(s *auth.Service) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			once.Do(func() {
				app = server.New(server.Config{StaticDir: dir, Probe: upProbe{}, Version: "test", Auth: s, References: w.svc})
			})
			app.ServeHTTP(rw, r)
		})
	})
	w.svc = reference.New(w.DB)
	return w
}

// frames builds n frames of a person standing still, every landmark seen.
func frames(n int) [][][]float64 {
	out := make([][][]float64, n)
	for f := range out {
		fr := make([][]float64, 33)
		for l := range fr {
			fr[l] = []float64{0.4 + 0.001*float64(f), 0.1 + 0.02*float64(l), 0.95, 0.01 * float64(l%7), -0.6 + 0.04*float64(l), 0.02}
		}
		out[f] = fr
	}
	return out
}

func seq(n int) pose.Sequence {
	return pose.Sequence{FPS: 30, Width: 1280, Height: 720, Frames: frames(n)}
}

func goodBody(shot, view string) map[string]any {
	return map[string]any{
		"shot": shot, "view": view, "player": "Test Pro", "handedness": "right",
		"source": "https://example.com/clip", "pose_sequence": seq(60),
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

type gate struct {
	Shot             string `json:"shot"`
	View             string `json:"view"`
	References       int    `json:"references"`
	LegacyReferences int    `json:"legacy_references"`
	Enabled          bool   `json:"enabled"`
}

type gates struct {
	MinReferences int    `json:"min_references"`
	Gates         []gate `json:"gates"`
}

// gateOf reads one shot + view from GET /api/admin/shot-gates.
func (w *world) gateOf(t *testing.T, c *http.Client, shot, view string) gate {
	t.Helper()
	res := w.Do(t, c, "GET", "/api/admin/shot-gates", "", "")
	if res.StatusCode != 200 {
		t.Fatalf("shot gates = %d", res.StatusCode)
	}
	g := decode[gates](t, res)
	if g.MinReferences != 3 {
		t.Errorf("min_references = %d, want 3", g.MinReferences)
	}
	if len(g.Gates) != 30 { // 15 shots x 2 views
		t.Errorf("%d gates, want 30", len(g.Gates))
	}
	for _, x := range g.Gates {
		if x.Shot == shot && x.View == view {
			return x
		}
	}
	t.Fatalf("no gate for %s %s", shot, view)
	return gate{}
}

type created struct {
	ID string `json:"id"`
}

func (w *world) add(t *testing.T, c *http.Client, csrf string, body map[string]any) created {
	t.Helper()
	res := w.Do(t, c, "POST", "/api/admin/references", mustJSON(t, body), csrf)
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("add reference = %d %s", res.StatusCode, b)
	}
	return decode[created](t, res)
}

func TestAdminAddsReferencesAndTheGateCounts(t *testing.T) {
	w := newWorld(t)
	c, me := w.As(t, auth.RoleAdmin)

	before := w.gateOf(t, c, "cut", "front_on")
	other := w.gateOf(t, c, "cut", "side_on")
	var ids []string
	for range 3 {
		ids = append(ids, w.add(t, c, me.CSRFToken, goodBody("cut", "front_on")).ID)
	}
	after := w.gateOf(t, c, "cut", "front_on")
	if after.References != before.References+3 {
		t.Errorf("cut front_on references %d -> %d, want +3", before.References, after.References)
	}
	if after.Enabled {
		t.Error("a gate must stay off until the leave-one-pro-out test (slice 5-6)")
	}
	if got := w.gateOf(t, c, "cut", "side_on"); got.References != other.References {
		t.Errorf("the other view changed: %d -> %d", other.References, got.References)
	}

	// The register lists them, without pose sequences.
	res := w.Do(t, c, "GET", "/api/admin/references", "", "")
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || strings.Contains(string(raw), "pose_sequence") {
		t.Fatalf("list = %d, body has pose_sequence: %v", res.StatusCode, strings.Contains(string(raw), "pose_sequence"))
	}
	for _, id := range ids {
		if !strings.Contains(string(raw), id) {
			t.Errorf("register misses %s", id)
		}
	}

	// One reference comes back with its pose sequence, for the stick figure.
	res = w.Do(t, c, "GET", "/api/admin/references/"+ids[0], "", "")
	if res.StatusCode != 200 {
		t.Fatalf("get = %d", res.StatusCode)
	}
	got := decode[struct {
		ID           string        `json:"id"`
		Shot         string        `json:"shot"`
		View         string        `json:"view"`
		Player       string        `json:"player"`
		Handedness   string        `json:"handedness"`
		Source       string        `json:"source"`
		Origin       string        `json:"origin"`
		FrameCount   int           `json:"frame_count"`
		PoseSequence pose.Sequence `json:"pose_sequence"`
	}](t, res)
	if got.ID != ids[0] || got.Shot != "cut" || got.View != "front_on" || got.Player != "Test Pro" ||
		got.Handedness != "right" || got.Source != "https://example.com/clip" || got.Origin != "browser" ||
		got.FrameCount != 60 || len(got.PoseSequence.Frames) != 60 {
		t.Errorf("reference = %+v (frames %d)", got, len(got.PoseSequence.Frames))
	}

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		res := w.Do(t, c, "GET", "/api/admin/references/"+id, "", "")
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("get %s = %d, want 404", id, res.StatusCode)
		}
	}
}

func TestConcurrentReferencesAreAllCounted(t *testing.T) {
	w := newWorld(t)
	c, me := w.As(t, auth.RoleAdmin)
	before := w.gateOf(t, c, "hook", "side_on")
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			res := w.Do(t, c, "POST", "/api/admin/references", mustJSON(t, goodBody("hook", "side_on")), me.CSRFToken)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusCreated {
				t.Errorf("add = %d", res.StatusCode)
			}
		})
	}
	wg.Wait()
	if got := w.gateOf(t, c, "hook", "side_on").References; got != before.References+6 {
		t.Errorf("references %d -> %d, want +6", before.References, got)
	}
}

func TestOldCSVReferencesAreKeptApartFromTheGate(t *testing.T) {
	w := newWorld(t)
	c, me := w.As(t, auth.RoleAdmin)
	var uid pgtype.UUID
	if err := uid.Scan(me.User.ID); err != nil {
		t.Fatal(err)
	}
	before := w.gateOf(t, c, "sweep", "side_on")
	in := reference.Input{
		Shot: "sweep", View: "side_on", Player: "Old Pro", Handedness: "right",
		Source: fmt.Sprintf("2026 hackathon CSV test-%s.csv", me.User.ID),
		Origin: store.ReferenceOriginLegacyCsv, Sequence: seq(30), CreatedBy: uid,
	}
	id, added, err := w.svc.Add(context.Background(), in)
	if err != nil || !added || id == "" {
		t.Fatalf("Add = %q, %v, %v", id, added, err)
	}
	after := w.gateOf(t, c, "sweep", "side_on")
	if after.References != before.References || after.LegacyReferences != before.LegacyReferences+1 {
		t.Errorf("gate %+v -> %+v: want the same count and one more old CSV reference", before, after)
	}
	// The import may run again: the same old file adds nothing.
	if _, added, err := w.svc.Add(context.Background(), in); err != nil || added {
		t.Errorf("second import of the same file: added=%v err=%v", added, err)
	}
	if got := w.gateOf(t, c, "sweep", "side_on"); got.LegacyReferences != after.LegacyReferences {
		t.Errorf("old CSV references %d -> %d after a second import", after.LegacyReferences, got.LegacyReferences)
	}
}

func TestStateChangesNeedTheCSRFToken(t *testing.T) {
	w := newWorld(t)
	c, me := w.As(t, auth.RoleAdmin)
	for _, token := range []string{"", "wrong-" + me.CSRFToken} {
		res := w.Do(t, c, "POST", "/api/admin/references", mustJSON(t, goodBody("pull", "side_on")), token)
		body := decode[map[string]string](t, res)
		if res.StatusCode != http.StatusForbidden || body["error"] != "csrf" {
			t.Errorf("token %q: %d %v, want 403 csrf", token, res.StatusCode, body)
		}
	}
}

func TestOnlyAdminsGetIn(t *testing.T) {
	w := newWorld(t)
	routes := []struct{ method, path, body string }{
		{"POST", "/api/admin/references", ""},
		{"GET", "/api/admin/references", ""},
		{"GET", "/api/admin/references/00000000-0000-0000-0000-000000000000", ""},
		{"GET", "/api/admin/shot-gates", ""},
	}
	routes[0].body = mustJSON(t, goodBody("pull", "side_on"))
	for _, role := range []auth.Role{auth.RoleNone, auth.RoleAthlete, auth.RoleCoach} {
		c, me := w.As(t, role)
		for _, rt := range routes {
			res := w.Do(t, c, rt.method, rt.path, rt.body, me.CSRFToken)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s as %q = %d, want 403", rt.method, rt.path, role, res.StatusCode)
			}
		}
	}
	anon := w.Client(t)
	for _, rt := range routes {
		res := w.Do(t, anon, rt.method, rt.path, rt.body, "")
		_ = res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s signed out = %d, want 401", rt.method, rt.path, res.StatusCode)
		}
	}
}

func TestBadReferencesAreRefused(t *testing.T) {
	w := newWorld(t)
	c, me := w.As(t, auth.RoleAdmin)
	with := func(k string, v any) string {
		b := goodBody("pull", "side_on")
		b[k] = v
		return mustJSON(t, b)
	}
	for _, tc := range []struct {
		name, body string
		code       int
		reason     string
	}{
		{"unknown shot", with("shot", "helicopter"), 422, "unknown_shot"},
		{"no shot", with("shot", ""), 422, "unknown_shot"},
		{"bad view", with("view", "top_down"), 422, "bad_view"},
		{"bad handedness", with("handedness", "both"), 422, "bad_handedness"},
		{"no player", with("player", "   "), 422, "bad_player"},
		{"long player", with("player", strings.Repeat("a", 81)), 422, "bad_player"},
		{"control character in player", with("player", "A\u0000B"), 422, "bad_player"},
		{"text direction trick in player", with("player", "Kohli\u202Egnp.exe"), 422, "bad_player"},
		{"SQL-looking player is just text", with("player", "' OR 1=1--"), 201, ""},
		{"no source", with("source", ""), 422, "bad_source"},
		{"long source", with("source", strings.Repeat("s", 501)), 422, "bad_source"},
		{"newline in source", with("source", "a\nb"), 422, "bad_source"},
		{"too few frames", with("pose_sequence", seq(5)), 422, "too_few_frames"},
		{"a video rides along", with("video", "AAAA"), 400, "bad_json"},
		{"not JSON", "{", 400, "bad_json"},
		{"too large", `{"source":"` + strings.Repeat("x", reference.MaxBodyBytes) + `"}`, 413, "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := w.Do(t, c, "POST", "/api/admin/references", tc.body, me.CSRFToken)
			body := decode[map[string]string](t, res)
			if res.StatusCode != tc.code || (tc.reason != "" && body["error"] != tc.reason) {
				t.Errorf("= %d %v, want %d %s", res.StatusCode, body, tc.code, tc.reason)
			}
		})
	}
}
