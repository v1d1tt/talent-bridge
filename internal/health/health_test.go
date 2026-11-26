package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/health"
)

// fakeProbe stands in for the database. It returns a fixed schema version or error.
type fakeProbe struct {
	version int64
	err     error
}

func (f fakeProbe) Probe(context.Context) (int64, error) { return f.version, f.err }

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		probe      health.Probe
		wantCode   int
		wantStatus string
		wantDB     string
		wantSchema int64
	}{
		{"database up", fakeProbe{version: 1}, http.StatusOK, "ok", "up", 1},
		{"database down", fakeProbe{err: errors.New("dial tcp: connection refused")}, http.StatusServiceUnavailable, "degraded", "down", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			health.Handler(tc.probe, "test-sha").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

			if rec.Code != tc.wantCode {
				t.Fatalf("status code = %d, want %d", rec.Code, tc.wantCode)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
			var got health.Report
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if got.Status != tc.wantStatus || got.Database.Status != tc.wantDB || got.Database.SchemaVersion != tc.wantSchema {
				t.Errorf("report = %+v, want status %q, database %q, schema %d", got, tc.wantStatus, tc.wantDB, tc.wantSchema)
			}
			if got.Version != "test-sha" {
				t.Errorf("version = %q, want test-sha", got.Version)
			}
		})
	}
}

func TestHandlerHidesErrorText(t *testing.T) {
	rec := httptest.NewRecorder()
	secret := "password authentication failed for user neondb_owner"
	health.Handler(fakeProbe{err: errors.New(secret)}, "x").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if body := rec.Body.String(); strings.Contains(body, "neondb_owner") {
		t.Errorf("response leaks the database error: %s", body)
	}
}
