// Package health serves GET /api/health: is the server up, and can it reach
// the database with the migrated schema?
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Probe checks the database. It returns the applied schema (migration) version.
type Probe interface {
	Probe(ctx context.Context) (schemaVersion int64, err error)
}

// Report is the JSON body of GET /api/health.
type Report struct {
	Status   string   `json:"status"`  // "ok" or "degraded"
	Version  string   `json:"version"` // git commit of the running build
	Database Database `json:"database"`
}

// Database is the database part of a Report.
type Database struct {
	Status        string `json:"status"` // "up" or "down"
	LatencyMS     int64  `json:"latency_ms"`
	SchemaVersion int64  `json:"schema_version"`
}

// probeTimeout is long enough for a Neon compute to wake from scale-to-zero.
const probeTimeout = 10 * time.Second

// Handler returns the health handler. It answers 200 when the database is up
// and 503 when it is not. The error text goes to the log, never to the client.
func Handler(p Probe, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
		defer cancel()

		start := time.Now()
		schema, err := p.Probe(ctx)
		rep := Report{
			Status:   "ok",
			Version:  version,
			Database: Database{Status: "up", LatencyMS: time.Since(start).Milliseconds(), SchemaVersion: schema},
		}
		code := http.StatusOK
		if err != nil {
			slog.ErrorContext(ctx, "health: database probe failed", "err", err)
			rep.Status, rep.Database.Status, rep.Database.SchemaVersion = "degraded", "down", 0
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(rep)
	})
}
