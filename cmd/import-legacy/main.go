// Command import-legacy imports the pose CSVs of the 2026 hackathon build
// into pro_references, once (slice 4, ADR 0008). The owner tagged each
// file (player, shot, view, handedness) on 2026-11-28; the tags are below.
//
//	DATABASE_URL=... go run ./cmd/import-legacy -admin-email you@example.com [-dir reference_data/cricket] [-dry-run]
//
// It is safe to run twice: a file imported before is skipped. The imported
// references are marked legacy_csv and do not count toward a shot gate.
//
// The import ran on 2026-11-28, and the files were then removed from the
// repo. To run it again (a new database), restore them from git history:
//
//	git checkout dffc74e -- reference_data/cricket
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/database"
	"github.com/v1d1tt/talent-bridge/internal/legacy"
	"github.com/v1d1tt/talent-bridge/internal/reference"
	"github.com/v1d1tt/talent-bridge/internal/store"
)

// tag is the owner's tagging of one old file.
type tag struct {
	file, player, shot, view, handedness string
}

// The 12 cricket files, tagged by the owner on 2026-11-28. The source
// videos are unknown.
var tags = []tag{
	{"Bumrah.csv", "Jasprit Bumrah", "fast_bowling", "front_on", "right"},
	{"Dale_Steyn.csv", "Dale Steyn", "fast_bowling", "front_on", "right"},
	{"Malinga.csv", "Lasith Malinga", "fast_bowling", "front_on", "right"},
	{"Starc.csv", "Mitchell Starc", "fast_bowling", "front_on", "left"},
	{"Shami.csv", "Mohammed Shami", "fast_bowling", "front_on", "right"},
	{"Bhubaneshwar.csv", "Bhuvneshwar Kumar", "fast_bowling", "front_on", "right"},
	{"Jadeja.csv", "Ravindra Jadeja", "off_spin", "front_on", "left"},
	{"Rashid.csv", "Rashid Khan", "leg_spin", "front_on", "right"},
	{"kohli_landmarks.csv", "Virat Kohli", "cover_drive", "side_on", "right"},
	{"Abd_landmarks.csv", "AB de Villiers", "sweep", "side_on", "right"},
	{"Rohit_Pull_Shot.csv", "Rohit Sharma", "pull", "side_on", "right"},
	{"Sachin_Straightdrive_Shot.csv", "Sachin Tendulkar", "straight_drive", "side_on", "right"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "import-legacy:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "reference_data/cricket", "folder with the old CSV files")
	email := flag.String("admin-email", "", "email of the admin the references are recorded under (required unless -dry-run)")
	dry := flag.Bool("dry-run", false, "convert and report, but do not write to the database")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var svc *reference.Service
	var admin pgtype.UUID
	if !*dry {
		url := os.Getenv("DATABASE_URL")
		if url == "" || *email == "" {
			return errors.New("set DATABASE_URL and -admin-email (or use -dry-run)")
		}
		db, err := database.Open(ctx, url)
		if err != nil {
			return err
		}
		defer db.Close()
		if _, err := db.Probe(ctx); err != nil { // the schema must be migrated
			return err
		}
		admin, err = db.Queries.GetAdminIDByEmail(ctx, *email)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no admin with the email %q: sign in on the app once first", *email)
		}
		if err != nil {
			return err
		}
		svc = reference.New(db)
	}

	fmt.Printf("%-30s %6s %5s %6s %6s %8s  %s\n", "file", "rows", "fps", "frames", "filled", "trimmed", "result")
	for _, t := range tags {
		f, err := os.Open(filepath.Join(*dir, t.file))
		if err != nil {
			return err
		}
		rows, err := legacy.Read(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", t.file, err)
		}
		res, err := legacy.Convert(rows)
		if err != nil {
			return fmt.Errorf("%s: %w", t.file, err)
		}
		result := "checked (dry run)"
		if svc != nil {
			_, added, err := svc.Add(ctx, reference.Input{
				Shot: t.shot, View: t.view, Player: t.player, Handedness: t.handedness,
				Source: "2026 hackathon CSV " + t.file + "; source video unknown",
				Origin: store.ReferenceOriginLegacyCsv, Sequence: res.Sequence, CreatedBy: admin,
			})
			switch {
			case err != nil:
				return fmt.Errorf("%s: %w", t.file, err)
			case added:
				result = "imported"
			default:
				result = "already imported"
			}
		}
		fmt.Printf("%-30s %6d %5g %6d %6d %7.1fs  %s\n", t.file, len(rows), res.Sequence.FPS,
			len(res.Sequence.Frames), res.FilledFrames, res.TrimmedSeconds, result)
	}
	return nil
}
