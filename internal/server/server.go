// Package server builds the HTTP handler: the JSON API under /api/ and the
// built Solid files for every other path.
package server

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/v1d1tt/talent-bridge/internal/analysis"
	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/health"
)

// Config is what the handler needs.
type Config struct {
	StaticDir string            // the Vite build output (web/dist)
	Probe     health.Probe      // the database check
	Version   string            // git commit of this build
	Auth      *auth.Service     // sign-in and access checks; nil means sign-in is off
	Analyses  *analysis.Service // nil means no database: the analysis routes answer 503
	// BlobOrigin is the Azure Blob Storage origin (https://<account>.blob.core.windows.net)
	// the page uploads key frames to and loads them from; empty when key frames are off.
	BlobOrigin string
}

// Route is one entry of the server's route table. Every route has an access
// rule; the zero rule denies everyone (deny by default).
type Route struct {
	Pattern string // method + path, as for http.ServeMux
	Access  auth.Access
	Handler http.HandlerFunc
}

// Routes is the route table: the only place that says who may call what.
// The route x role test (server_test.go) fails when a route here has no row.
func Routes(cfg Config) []Route {
	a, an := cfg.Auth, cfg.Analyses
	athlete := auth.Roles(auth.RoleAthlete)
	return []Route{
		{"GET /api/health", auth.Public, health.Handler(cfg.Probe, cfg.Version).ServeHTTP},
		{"GET /auth/google/start", auth.Public, a.Start},
		{"GET " + auth.CallbackPath, auth.Public, a.Callback},
		{"POST /auth/signout", auth.SignedIn, a.SignOut},
		{"GET /api/me", auth.Optional, a.Me},
		{"POST /api/me/role", auth.SignedIn, a.ChooseRole},
		// Analyses belong to athletes. Get and the key frame check also check
		// the owner: another athlete's id answers 404.
		{"POST /api/analyses", athlete, an.Create},
		{"GET /api/analyses/{id}", athlete, an.Get},
		{"POST /api/analyses/{id}/key-frames/check", athlete, an.CheckKeyFrames},
	}
}

// New returns the app's root handler. Routing uses the Go standard library
// (method + path patterns, Go 1.22+); see docs/learning/01.md for why not chi.
func New(cfg Config) http.Handler {
	if cfg.Auth == nil {
		cfg.Auth = auth.Disabled()
	}
	if cfg.Analyses == nil {
		cfg.Analyses = analysis.Disabled()
	}
	mux := http.NewServeMux()
	paths := map[string]bool{}
	for _, rt := range Routes(cfg) {
		mux.Handle(rt.Pattern, cfg.Auth.Guard(rt.Access, rt.Handler))
		_, p, _ := strings.Cut(rt.Pattern, " ")
		paths[p] = true
	}
	// The catch-alls below would answer other methods on a known path with
	// 404, so each known path names its wrong methods explicitly.
	for p := range paths {
		mux.HandleFunc(p, jsonError(http.StatusMethodNotAllowed, "method_not_allowed"))
	}
	mux.HandleFunc("/api/", jsonError(http.StatusNotFound, "not_found"))
	mux.HandleFunc("/auth/", jsonError(http.StatusNotFound, "not_found"))
	mux.Handle("/", spa(cfg.StaticDir))
	return securityHeaders(mux, contentSecurityPolicy(cfg.BlobOrigin))
}

func jsonError(code int, msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"` + msg + `"}` + "\n"))
	}
}

// spa serves files from dir. A path with no file extension that does not
// exist is a client-side route, so it gets index.html. A missing file with
// an extension (a stale asset) is a real 404.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clean)))
		switch {
		case err == nil && clean != "/":
			if strings.HasPrefix(clean, "/assets/") || strings.HasPrefix(clean, "/mediapipe/") {
				// Vite puts a content hash in every asset name, and the MediaPipe
				// files sit under their version number, so they never change.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			if servePrecompressed(w, r, dir, clean) {
				return
			}
			files.ServeHTTP(w, r)
		case clean == "/" || (errors.Is(err, fs.ErrNotExist) && path.Ext(clean) == ""):
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		default:
			http.NotFound(w, r)
		}
	})
}

// servePrecompressed serves name.gz, made at build time, to a browser that
// accepts gzip. It reports whether it did.
func servePrecompressed(w http.ResponseWriter, r *http.Request, dir, name string) bool {
	gz := filepath.Join(dir, filepath.FromSlash(name)+".gz")
	if _, err := os.Stat(gz); err != nil {
		return false
	}
	w.Header().Add("Vary", "Accept-Encoding")
	if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	f, err := os.Open(gz)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Encoding", "gzip")
	http.ServeContent(w, r, name, st.ModTime(), f)
	return true
}

func acceptsGzip(header string) bool {
	for part := range strings.SplitSeq(header, ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(coding), "gzip") && strings.ReplaceAll(params, " ", "") != "q=0" {
			return true
		}
	}
	return false
}

// contentSecurityPolicy allows only this origin, plus Blob Storage for the
// key frames (PUT from the page, <img> on the result page). The pose model
// runs as WebAssembly, which needs 'wasm-unsafe-eval' (it allows compiling
// WebAssembly, not JavaScript eval). Fonts are the system's own.
func contentSecurityPolicy(blobOrigin string) string {
	blob := ""
	if blobOrigin != "" {
		blob = " " + blobOrigin
	}
	return "default-src 'self'; img-src 'self' data: blob:" + blob + "; style-src 'self'; " +
		"script-src 'self' 'wasm-unsafe-eval'; connect-src 'self'" + blob + "; media-src 'self' blob:; " +
		"font-src 'self'; object-src 'none'; base-uri 'self'; " +
		"form-action 'self'; frame-ancestors 'none'"
}

func securityHeaders(next http.Handler, csp string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		// Browsers ignore HSTS on plain-HTTP responses, so this is safe on localhost.
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
