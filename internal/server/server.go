// Package server builds the HTTP handler: the JSON API under /api/ and the
// built Solid files for every other path.
package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/v1d1tt/talent-bridge/internal/health"
)

// Config is what the handler needs.
type Config struct {
	StaticDir string       // the Vite build output (web/dist)
	Probe     health.Probe // the database check
	Version   string       // git commit of this build
}

// New returns the app's root handler. Routing uses the Go standard library
// (method + path patterns, Go 1.22+); see docs/learning/01.md for why not chi.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/health", health.Handler(cfg.Probe, cfg.Version))
	// The /api/ catch-all would answer other methods on a known path with 404,
	// so a known path names its wrong methods explicitly.
	mux.HandleFunc("/api/health", jsonError(http.StatusMethodNotAllowed, "method_not_allowed"))
	mux.HandleFunc("/api/", jsonError(http.StatusNotFound, "not_found"))
	mux.Handle("/", spa(cfg.StaticDir))
	return securityHeaders(mux)
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
			if strings.HasPrefix(clean, "/assets/") {
				// Vite puts a content hash in every asset name, so they never change.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
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

// csp allows only this origin. Fonts are self-hosted, so no third-party host
// is needed.
const csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; " +
	"connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'self'; " +
	"form-action 'self'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
