package database_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/database"
)

// These tests need a real, empty-or-migrated Postgres. CI starts one as a
// service container; locally `make test` starts one in Docker.
func testURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	return url
}

func TestProbeMigratesAndReportsSchemaVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.Open(ctx, testURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Many first requests at once (a cold start) must apply the migrations once.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			v, err := db.Probe(ctx)
			if err == nil && v != 3 {
				t.Errorf("schema version = %d, want 3", v)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestProbeFailsWhenDatabaseIsUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := database.Open(ctx, "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err) // Open must not connect, so it must not fail here.
	}
	defer db.Close()

	if _, err := db.Probe(ctx); err == nil {
		t.Fatal("Probe() = nil error, want an error for an unreachable database")
	}
}

func TestOpenRejectsABadURL(t *testing.T) {
	if _, err := database.Open(context.Background(), "::not a url::"); err == nil {
		t.Fatal("Open() = nil error, want an error for a malformed URL")
	}
}
