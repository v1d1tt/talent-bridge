package analysis_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/analysis"
	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/auth/authtest"
	"github.com/v1d1tt/talent-bridge/internal/blob"
	"github.com/v1d1tt/talent-bridge/internal/server"
)

type upProbe struct{}

func (upProbe) Probe(context.Context) (int64, error) { return 1, nil }

// world is the app with a database, the fake Google and (when
// TEST_BLOB_URL is set, else the test skips) Azurite for key frames.
type world struct {
	*authtest.Env
	svc *analysis.Service
}

func newWorld(t *testing.T, withBlobs bool) *world {
	t.Helper()
	var store *blob.Store
	if withBlobs {
		u := os.Getenv("TEST_BLOB_URL")
		if u == "" {
			t.Skip("TEST_BLOB_URL is not set")
		}
		cfg := blob.Config{AccountURL: u, Container: "t-" + strings.ToLower(rand.Text()[:12]), AccountKey: blob.AzuriteKey}
		if err := blob.SetupDev(context.Background(), cfg, []string{"https://localhost"}); err != nil {
			t.Fatal(err)
		}
		var err error
		if store, err = blob.New(cfg); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &world{}
	// The analysis service needs the database, which authtest.New opens, so
	// the app's handler is built at its first request.
	var once sync.Once
	var app http.Handler
	w.Env = authtest.New(t, func(s *auth.Service) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			once.Do(func() {
				app = server.New(server.Config{StaticDir: dir, Probe: upProbe{}, Version: "test", Auth: s, Analyses: w.svc})
			})
			app.ServeHTTP(rw, r)
		})
	})
	w.svc = analysis.New(w.DB, store)
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

type body struct {
	PoseSequence any   `json:"pose_sequence"`
	KeyFrames    []any `json:"key_frames"`
}

func goodBody() body {
	return body{
		PoseSequence: map[string]any{"fps": 30, "width": 1280, "height": 720, "frames": frames(60)},
		KeyFrames:    []any{map[string]int{"frame_index": 10}, map[string]int{"frame_index": 30}, map[string]int{"frame_index": 50}},
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

type created struct {
	ID        string `json:"id"`
	KeyFrames []struct {
		FrameIndex int       `json:"frame_index"`
		UploadURL  string    `json:"upload_url"`
		ExpiresAt  time.Time `json:"expires_at"`
	} `json:"key_frames"`
}

type result struct {
	ID           string `json:"id"`
	FPS          float64
	FrameCount   int     `json:"frame_count"`
	Duration     float64 `json:"duration_s"`
	Score        *int    `json:"score"`
	PoseSequence struct {
		FPS    float64       `json:"fps"`
		Width  int           `json:"width"`
		Height int           `json:"height"`
		Frames [][][]float64 `json:"frames"`
	} `json:"pose_sequence"`
	KeyFrames []struct {
		FrameIndex int    `json:"frame_index"`
		State      string `json:"state"`
		URL        string `json:"url"`
	} `json:"key_frames"`
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

func putJPEG(t *testing.T, u string, b []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, u, bytes.NewReader(b))
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", "image/jpeg")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("PUT key frame = %d", res.StatusCode)
	}
}

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xDB}, bytes.Repeat([]byte{7}, 2000)...)

func TestCreateCheckAndGetAnalysis(t *testing.T) {
	w := newWorld(t, true)
	c, me := w.As(t, auth.RoleAthlete)

	res := w.Do(t, c, http.MethodPost, "/api/analyses", mustJSON(t, goodBody()), me.CSRFToken)
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("POST /api/analyses = %d %s, want 201", res.StatusCode, b)
	}
	cr := decode[created](t, res)
	if len(cr.KeyFrames) != 3 || cr.KeyFrames[0].FrameIndex != 10 || cr.KeyFrames[0].UploadURL == "" {
		t.Fatalf("key frames = %+v, want 3 with upload URLs", cr.KeyFrames)
	}

	// Before any upload: all pending, no read URLs.
	got := decode[result](t, w.Do(t, c, http.MethodGet, "/api/analyses/"+cr.ID, "", ""))
	for _, k := range got.KeyFrames {
		if k.State != "pending" || k.URL != "" {
			t.Errorf("before upload: key frame %+v, want pending with no URL", k)
		}
	}

	putJPEG(t, cr.KeyFrames[0].UploadURL, jpeg)                                      // good
	putJPEG(t, cr.KeyFrames[1].UploadURL, []byte("<html><script>x</script></html>")) // not a JPEG
	// The third is never uploaded.

	res = w.Do(t, c, http.MethodPost, "/api/analyses/"+cr.ID+"/key-frames/check", "", me.CSRFToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("check = %d, want 200", res.StatusCode)
	}
	got = decode[result](t, res)
	want := []string{"uploaded", "rejected", "pending"}
	for i, k := range got.KeyFrames {
		if k.State != want[i] {
			t.Errorf("key frame %d state = %q, want %q", k.FrameIndex, k.State, want[i])
		}
		if (k.URL != "") != (want[i] == "uploaded") {
			t.Errorf("key frame %d url = %q; only uploaded ones get a read URL", k.FrameIndex, k.URL)
		}
	}
	// The read URL serves the JPEG; the rejected blob is gone.
	r2, err := http.Get(got.KeyFrames[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r2.Body)
	_ = r2.Body.Close()
	if r2.StatusCode != 200 || !bytes.Equal(b, jpeg) {
		t.Errorf("read URL = %d (%d bytes), want the JPEG", r2.StatusCode, len(b))
	}

	got = decode[result](t, w.Do(t, c, http.MethodGet, "/api/analyses/"+cr.ID, "", ""))
	if got.FrameCount != 60 || got.FPS != 30 || got.Duration != 2 || got.Score != nil {
		t.Errorf("analysis = %d frames, %v fps, %v s, score %v; want 60, 30, 2, null", got.FrameCount, got.FPS, got.Duration, got.Score)
	}
	if len(got.PoseSequence.Frames) != 60 || got.PoseSequence.Width != 1280 || len(got.PoseSequence.Frames[59]) != 33 {
		t.Fatalf("pose sequence came back with %d frames, width %d", len(got.PoseSequence.Frames), got.PoseSequence.Width)
	}
	if v := got.PoseSequence.Frames[59][32][1]; v < 0.739 || v > 0.741 {
		t.Errorf("frame 59 landmark 32 y = %v, want 0.74", v)
	}
}

func TestOnlyTheOwnerCanSeeAnAnalysis(t *testing.T) {
	w := newWorld(t, true)
	owner, me := w.As(t, auth.RoleAthlete)
	cr := decode[created](t, w.Do(t, owner, http.MethodPost, "/api/analyses", mustJSON(t, goodBody()), me.CSRFToken))
	other, otherMe := w.As(t, auth.RoleAthlete)
	coach, coachMe := w.As(t, auth.RoleCoach)
	admin, adminMe := w.As(t, auth.RoleAdmin)

	for _, tc := range []struct {
		name   string
		c      *http.Client
		csrf   string
		method string
		path   string
		want   int
	}{
		{"owner reads", owner, "", "GET", "/api/analyses/" + cr.ID, 200},
		{"another athlete reads", other, "", "GET", "/api/analyses/" + cr.ID, 404},
		{"another athlete checks key frames", other, otherMe.CSRFToken, "POST", "/api/analyses/" + cr.ID + "/key-frames/check", 404},
		{"coach reads", coach, "", "GET", "/api/analyses/" + cr.ID, 403},
		{"admin reads", admin, "", "GET", "/api/analyses/" + cr.ID, 403},
		{"signed out reads", w.Client(t), "", "GET", "/api/analyses/" + cr.ID, 401},
		{"coach creates", coach, coachMe.CSRFToken, "POST", "/api/analyses", 403},
		{"admin creates", admin, adminMe.CSRFToken, "POST", "/api/analyses", 403},
		{"not a uuid", owner, "", "GET", "/api/analyses/not-a-uuid", 404},
		{"unknown id", owner, "", "GET", "/api/analyses/00000000-0000-4000-8000-000000000000", 404},
		{"create without CSRF token", owner, "", "POST", "/api/analyses", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := ""
			if tc.method == "POST" && strings.HasSuffix(tc.path, "/analyses") {
				b = mustJSON(t, goodBody())
			}
			res := w.Do(t, tc.c, tc.method, tc.path, b, tc.csrf)
			_ = res.Body.Close()
			if res.StatusCode != tc.want {
				t.Errorf("%s %s = %d, want %d", tc.method, tc.path, res.StatusCode, tc.want)
			}
		})
	}
}

// The HTTP side of every refusal: status, error code and a message. The
// pose package's own table tests cover each shape rule in detail.
func TestCreateRefusesBadRequests(t *testing.T) {
	w := newWorld(t, true)
	c, me := w.As(t, auth.RoleAthlete)

	withPose := func(change func(map[string]any)) string {
		b := goodBody()
		p := b.PoseSequence.(map[string]any)
		change(p)
		return mustJSON(t, b)
	}
	withKeyFrames := func(idx ...int) string {
		b := goodBody()
		b.KeyFrames = nil
		for _, i := range idx {
			b.KeyFrames = append(b.KeyFrames, map[string]int{"frame_index": i})
		}
		return mustJSON(t, b)
	}
	broken := func(f, l int, row []float64) func(map[string]any) {
		return func(p map[string]any) {
			fr := frames(60)
			if row == nil {
				fr[f] = fr[f][:l]
			} else {
				fr[f][l] = row
			}
			p["frames"] = fr
		}
	}
	tests := []struct {
		name string
		body string
		code int
		err  string
	}{
		{"not JSON", "{frames", 400, "bad_json"},
		{"unknown field", `{"pose_sequence":{},"video":"AAAA"}`, 400, "bad_json"},
		{"no pose sequence", `{"key_frames":[]}`, 422, "too_few_frames"},
		{"14 frames", withPose(func(p map[string]any) { p["frames"] = frames(14) }), 422, "too_few_frames"},
		{"601 frames", withPose(func(p map[string]any) { p["fps"] = 60; p["frames"] = frames(601) }), 422, "too_many_frames"},
		{"fps 10", withPose(func(p map[string]any) { p["fps"] = 10 }), 422, "fps_out_of_range"},
		{"fps 120", withPose(func(p map[string]any) { p["fps"] = 120 }), 422, "fps_out_of_range"},
		{"0.5 s", withPose(func(p map[string]any) { p["fps"] = 60; p["frames"] = frames(30) }), 422, "clip_too_short"},
		{"11 s", withPose(func(p map[string]any) { p["fps"] = 15; p["frames"] = frames(165) }), 422, "clip_too_long"},
		{"no width", withPose(func(p map[string]any) { delete(p, "width") }), 422, "bad_video_size"},
		{"32 landmarks", withPose(broken(5, 32, nil)), 422, "wrong_landmark_count"},
		{"4 values", withPose(broken(5, 3, []float64{0.5, 0.5, 1, 0})), 422, "wrong_value_count"},
		{"x is 9", withPose(broken(5, 3, []float64{9, 0.5, 1, 0, 0, 0})), 422, "value_out_of_range"},
		{"world z is 40 m", withPose(broken(5, 3, []float64{0.5, 0.5, 1, 0, 0, 40})), 422, "value_out_of_range"},
		{"nobody in the clip", withPose(func(p map[string]any) {
			fr := frames(60)
			for f := range fr {
				for l := range fr[f] {
					fr[f][l] = []float64{0, 0, 0, 0, 0, 0}
				}
			}
			p["frames"] = fr
		}), 422, "person_not_found"},
		{"one key frame", withKeyFrames(10), 422, "key_frame_count"},
		{"four key frames", withKeyFrames(1, 2, 3, 4), 422, "key_frame_count"},
		{"key frame past the end", withKeyFrames(10, 60), 422, "key_frame_index"},
		{"negative key frame", withKeyFrames(-1, 10), 422, "key_frame_index"},
		{"same key frame twice", withKeyFrames(10, 10), 422, "key_frame_index"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := w.Do(t, c, http.MethodPost, "/api/analyses", tc.body, me.CSRFToken)
			e := decode[struct{ Error, Message string }](t, res)
			if res.StatusCode != tc.code || e.Error != tc.err {
				t.Errorf("= %d %q (%s), want %d %q", res.StatusCode, e.Error, e.Message, tc.code, tc.err)
			}
			if tc.code == 422 && e.Message == "" {
				t.Errorf("422 %q has no message", e.Error)
			}
		})
	}

	t.Run("body over 4 MiB", func(t *testing.T) {
		big := `{"pose_sequence":{"fps":30,"frames":[` + strings.Repeat(`[[0.1,0.1,1,0,0,0]],`, 230000) + `[]]}}`
		res := w.Do(t, c, http.MethodPost, "/api/analyses", big, me.CSRFToken)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("= %d, want 413", res.StatusCode)
		}
	})
}

func TestDailyLimit(t *testing.T) {
	w := newWorld(t, false)
	w.svc.DailyLimit = 2
	c, me := w.As(t, auth.RoleAthlete)
	for i, want := range []int{201, 201, 429} {
		res := w.Do(t, c, http.MethodPost, "/api/analyses", mustJSON(t, goodBody()), me.CSRFToken)
		_ = res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("analysis %d = %d, want %d", i+1, res.StatusCode, want)
		}
	}
}

func TestWithoutBlobStorageKeyFramesAreOff(t *testing.T) {
	w := newWorld(t, false)
	c, me := w.As(t, auth.RoleAthlete)
	res := w.Do(t, c, http.MethodPost, "/api/analyses", mustJSON(t, goodBody()), me.CSRFToken)
	if res.StatusCode != 201 {
		t.Fatalf("= %d, want 201", res.StatusCode)
	}
	cr := decode[created](t, res)
	if len(cr.KeyFrames) != 0 {
		t.Errorf("key frames = %v, want none when storage is off", cr.KeyFrames)
	}
	got := decode[result](t, w.Do(t, c, http.MethodGet, fmt.Sprintf("/api/analyses/%s", cr.ID), "", ""))
	if got.FrameCount != 60 || len(got.KeyFrames) != 0 {
		t.Errorf("GET = %d frames, %d key frames", got.FrameCount, len(got.KeyFrames))
	}
}

func TestCheckRejectsTooLargeOrTooSmallKeyFrames(t *testing.T) {
	w := newWorld(t, true)
	c, me := w.As(t, auth.RoleAthlete)
	b := goodBody()
	b.KeyFrames = b.KeyFrames[:2]
	cr := decode[created](t, w.Do(t, c, http.MethodPost, "/api/analyses", mustJSON(t, b), me.CSRFToken))
	putJPEG(t, cr.KeyFrames[0].UploadURL, append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, analysis.MaxKeyFrameBytes)...))
	putJPEG(t, cr.KeyFrames[1].UploadURL, []byte{0xFF, 0xD8, 0xFF, 0xD9})
	got := decode[result](t, w.Do(t, c, http.MethodPost, "/api/analyses/"+cr.ID+"/key-frames/check", "", me.CSRFToken))
	for _, k := range got.KeyFrames {
		if k.State != "rejected" || k.URL != "" {
			t.Errorf("key frame %d = %q %q, want rejected with no URL", k.FrameIndex, k.State, k.URL)
		}
	}
}

// After the upload window closes, the sweep settles every key frame once:
// a good JPEG that nobody checked becomes uploaded, a missing one is
// rejected, and junk is deleted, even junk uploaded again after a
// rejection (the upload URL still worked until it expired).
func TestSweepSettlesKeyFramesAfterTheUploadWindow(t *testing.T) {
	w := newWorld(t, true)
	c, me := w.As(t, auth.RoleAthlete)
	cr := decode[created](t, w.Do(t, c, http.MethodPost, "/api/analyses", mustJSON(t, goodBody()), me.CSRFToken))

	putJPEG(t, cr.KeyFrames[0].UploadURL, []byte("junk, not a picture, long enough to pass the size floor......................................................"))
	res := w.Do(t, c, http.MethodPost, "/api/analyses/"+cr.ID+"/key-frames/check", "", me.CSRFToken)
	_ = res.Body.Close()
	putJPEG(t, cr.KeyFrames[0].UploadURL, []byte("junk again")) // allowed: the blob was deleted, the URL is still valid
	putJPEG(t, cr.KeyFrames[1].UploadURL, jpeg)                 // good, but the browser never asked for a check
	// frame 2: never uploaded

	ctx := context.Background()
	// Before the window closes, the sweep leaves everything alone.
	if n, err := w.svc.Sweep(ctx, time.Now()); err != nil || n != 0 {
		t.Fatalf("Sweep inside the window = %d, %v; want 0, nil", n, err)
	}
	// The test database is shared, so key frames of earlier tests are settled too.
	n, err := w.svc.Sweep(ctx, time.Now().Add(blob.UploadTTL+time.Minute))
	if err != nil || n < 3 {
		t.Fatalf("Sweep after the window = %d, %v; want at least 3, nil", n, err)
	}
	got := decode[result](t, w.Do(t, c, http.MethodGet, "/api/analyses/"+cr.ID, "", ""))
	want := []string{"rejected", "uploaded", "rejected"}
	for i, k := range got.KeyFrames {
		if k.State != want[i] {
			t.Errorf("key frame %d = %q, want %q", k.FrameIndex, k.State, want[i])
		}
	}
	if _, err := w.svc.BlobsForTest().Inspect(ctx, "analyses/"+cr.ID+"/10.jpg"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("re-uploaded junk is still there (Inspect = %v)", err)
	}
	// A second sweep finds nothing left to do.
	if n, err := w.svc.Sweep(ctx, time.Now().Add(blob.UploadTTL+time.Minute)); err != nil || n != 0 {
		t.Errorf("second Sweep = %d, %v; want 0, nil", n, err)
	}
}
