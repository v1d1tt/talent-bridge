// Package reference stores pro references and serves the admin's
// reference register and shot gates (PRD 3.11, GLOSSARY "Reference").
//
//	POST /api/admin/references        admin: tags + pose sequence -> new reference
//	GET  /api/admin/references        admin: the register (no pose sequences)
//	GET  /api/admin/references/{id}   admin: one reference with its pose sequence
//	GET  /api/admin/shot-gates        admin: reference count per shot + view
//
// The admin page makes the pose sequence with the same browser code as an
// athlete's analysis (ADR 0001). The clip never reaches the server, and no
// key frame of a pro clip is stored: pro footage is not ours to keep.
package reference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/database"
	"github.com/v1d1tt/talent-bridge/internal/pose"
	"github.com/v1d1tt/talent-bridge/internal/store"
)

// Limits.
const (
	MaxBodyBytes  = 4 << 20 // same as an analysis: 600 frames of landmarks
	MaxPlayer     = 80
	MaxSource     = 500
	MinReferences = 3 // the shot gate needs 3+ browser references (GLOSSARY "Shot gate")
)

// Service holds the database.
type Service struct {
	db *database.DB
}

// New returns the service.
func New(db *database.DB) *Service { return &Service{db: db} }

// Disabled answers 503 on every route: the server has no database.
func Disabled() *Service { return &Service{} }

// Input is one new reference.
type Input struct {
	Shot       string // shot code, for example "fast_bowling"
	View       string // side_on | front_on
	Player     string
	Handedness string // right | left
	Source     string // a URL or a note on where the clip came from
	Origin     store.ReferenceOrigin
	Sequence   pose.Sequence
	CreatedBy  pgtype.UUID
}

// InputError is a reference the service refuses. Reason is a stable code
// for the page; Message is plain English.
type InputError struct{ Reason, Message string }

func (e *InputError) Error() string { return e.Reason + ": " + e.Message }

func refuse(reason, format string, a ...any) *InputError {
	return &InputError{Reason: reason, Message: fmt.Sprintf(format, a...)}
}

// check validates the tags and the pose sequence. It trims the player and
// the source in place.
func (in *Input) check() error {
	in.Player = strings.TrimSpace(in.Player)
	in.Source = strings.TrimSpace(in.Source)
	switch {
	case in.View != string(store.ClipViewSideOn) && in.View != string(store.ClipViewFrontOn):
		return refuse("bad_view", "The view must be side_on or front_on.")
	case in.Handedness != string(store.HandednessRight) && in.Handedness != string(store.HandednessLeft):
		return refuse("bad_handedness", "Handedness must be right or left.")
	case !plainText(in.Player, MaxPlayer):
		return refuse("bad_player", "Give the player's name: 1 to %d characters, on one line.", MaxPlayer)
	case !plainText(in.Source, MaxSource):
		return refuse("bad_source", "Give the source (a URL or a note): 1 to %d characters, on one line.", MaxSource)
	}
	if err := pose.Validate(in.Sequence); err != nil {
		var pe *pose.Error
		if errors.As(err, &pe) {
			return refuse(pe.Reason, "%s", pe.Message)
		}
		return err
	}
	return nil
}

// plainText is 1..max characters of valid UTF-8 with no control characters
// (no newlines, no NUL) and no text-direction controls (such as U+202E,
// which would make a name read backwards on screen).
func plainText(s string, max int) bool {
	n := utf8.RuneCountInString(s)
	if n < 1 || n > max || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) })
}

// ErrUnavailable means the server has no database.
var ErrUnavailable = errors.New("reference: no database")

// Add checks and stores a reference, and recounts its shot gate in the
// same transaction. added is false when an old CSV with the same source
// was imported before (the import may run twice).
func (s *Service) Add(ctx context.Context, in Input) (id string, added bool, err error) {
	if s.db == nil {
		return "", false, ErrUnavailable
	}
	if err := in.check(); err != nil {
		return "", false, err
	}
	shot, err := s.db.Queries.GetShotByCode(ctx, in.Shot)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, refuse("unknown_shot", "%q is not a shot in the list.", in.Shot)
	}
	if err != nil {
		return "", false, err
	}
	encoded, err := pose.Encode(in.Sequence)
	if err != nil {
		return "", false, err
	}
	view := store.ClipView(in.View)
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		gate := store.LockShotGateParams{ShotID: shot.ID, View: view}
		if _, err := q.LockShotGate(ctx, gate); err != nil {
			return fmt.Errorf("lock shot gate: %w", err)
		}
		row, err := q.CreateReference(ctx, store.CreateReferenceParams{
			ShotID: shot.ID, View: view, Player: in.Player, Handedness: store.Handedness(in.Handedness),
			Source: in.Source, Origin: in.Origin, Fps: float32(in.Sequence.FPS),
			FrameCount: int32(len(in.Sequence.Frames)), PoseSequence: encoded, CreatedBy: in.CreatedBy,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // an old CSV imported before
		}
		if err != nil {
			return err
		}
		id, added = row.ID.String(), true
		return q.RecountShotGate(ctx, store.RecountShotGateParams{ShotID: shot.ID, View: view})
	})
	return id, added, err
}

// createRequest is the body of POST /api/admin/references. Unknown fields
// are refused, so nothing else (a video, an image) can ride along.
type createRequest struct {
	Shot         string        `json:"shot"`
	View         string        `json:"view"`
	Player       string        `json:"player"`
	Handedness   string        `json:"handedness"`
	Source       string        `json:"source"`
	PoseSequence pose.Sequence `json:"pose_sequence"`
}

// Create serves POST /api/admin/references. Guard it with Roles(admin).
func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return
	}
	sess, _ := auth.SessionFrom(r.Context())
	var uid pgtype.UUID
	if sess == nil || uid.Scan(sess.User.ID) != nil {
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("The request is larger than %d MB.", MaxBodyBytes>>20))
			return
		}
		writeError(w, http.StatusBadRequest, "bad_json", "The request is not valid JSON for this endpoint.")
		return
	}
	id, _, err := s.Add(r.Context(), Input{
		Shot: req.Shot, View: req.View, Player: req.Player, Handedness: req.Handedness, Source: req.Source,
		Origin: store.ReferenceOriginBrowser, Sequence: req.PoseSequence, CreatedBy: uid,
	})
	var ie *InputError
	switch {
	case errors.As(err, &ie):
		writeError(w, http.StatusUnprocessableEntity, ie.Reason, ie.Message)
		return
	case err != nil:
		internal(w, r, "add reference", err)
		return
	}
	slog.InfoContext(r.Context(), "reference: created", "admin", sess.User.ID, "reference", id,
		"shot", req.Shot, "view", req.View, "frames", len(req.PoseSequence.Frames))
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

type referenceJSON struct {
	ID           string         `json:"id"`
	Shot         string         `json:"shot"`
	ShotName     string         `json:"shot_name"`
	Discipline   string         `json:"discipline"`
	View         string         `json:"view"`
	Player       string         `json:"player"`
	Handedness   string         `json:"handedness"`
	Source       string         `json:"source"`
	Origin       string         `json:"origin"` // browser | legacy_csv
	FPS          float64        `json:"fps"`
	FrameCount   int            `json:"frame_count"`
	Duration     float64        `json:"duration_s"`
	CreatedAt    time.Time      `json:"created_at"`
	PoseSequence *pose.Sequence `json:"pose_sequence,omitempty"`
}

func duration(frames int32, fps float32) float64 {
	return math.Round(float64(frames)/float64(fps)*100) / 100
}

// List serves GET /api/admin/references: the reference register.
func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return
	}
	rows, err := s.db.Queries.ListReferences(r.Context())
	if err != nil {
		internal(w, r, "list references", err)
		return
	}
	out := make([]referenceJSON, 0, len(rows))
	for _, x := range rows {
		out = append(out, referenceJSON{
			ID: x.ID.String(), Shot: x.ShotCode, ShotName: x.ShotName, Discipline: string(x.Discipline),
			View: string(x.View), Player: x.Player, Handedness: string(x.Handedness), Source: x.Source,
			Origin: string(x.Origin), FPS: float64(x.Fps), FrameCount: int(x.FrameCount),
			Duration: duration(x.FrameCount, x.Fps), CreatedAt: x.CreatedAt.Time.UTC(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"references": out})
}

// Get serves GET /api/admin/references/{id}, with the pose sequence.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return
	}
	var id pgtype.UUID
	if raw := r.PathValue("id"); len(raw) != 36 || id.Scan(raw) != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	x, err := s.db.Queries.GetReference(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	if err != nil {
		internal(w, r, "get reference", err)
		return
	}
	seq, err := pose.Decode(x.PoseSequence)
	if err != nil {
		internal(w, r, "decode pose sequence", err)
		return
	}
	writeJSON(w, http.StatusOK, referenceJSON{
		ID: x.ID.String(), Shot: x.ShotCode, ShotName: x.ShotName, Discipline: string(x.Discipline),
		View: string(x.View), Player: x.Player, Handedness: string(x.Handedness), Source: x.Source,
		Origin: string(x.Origin), FPS: seq.FPS, FrameCount: int(x.FrameCount),
		Duration: duration(x.FrameCount, x.Fps), CreatedAt: x.CreatedAt.Time.UTC(), PoseSequence: &seq,
	})
}

type gateJSON struct {
	Shot             string `json:"shot"`
	ShotName         string `json:"shot_name"`
	Discipline       string `json:"discipline"`
	View             string `json:"view"`
	References       int    `json:"references"`        // browser references: these count
	LegacyReferences int    `json:"legacy_references"` // old CSV references: shown, not counted
	LopoPassed       bool   `json:"lopo_passed"`
	Enabled          bool   `json:"enabled"`
}

// Gates serves GET /api/admin/shot-gates: one row per shot + view.
func (s *Service) Gates(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return
	}
	rows, err := s.db.Queries.ListShotGates(r.Context())
	if err != nil {
		internal(w, r, "list shot gates", err)
		return
	}
	out := make([]gateJSON, 0, len(rows))
	for _, g := range rows {
		out = append(out, gateJSON{
			Shot: g.ShotCode, ShotName: g.ShotName, Discipline: string(g.Discipline), View: string(g.View),
			References: int(g.ReferenceCount), LegacyReferences: int(g.LegacyCount),
			LopoPassed: g.LopoPassed, Enabled: g.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"min_references": MinReferences, "gates": out})
}

func internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	if errors.Is(err, context.Canceled) {
		return // the browser went away
	}
	slog.ErrorContext(r.Context(), "reference: "+what, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, reason, message string) {
	body := map[string]string{"error": reason}
	if message != "" {
		body["message"] = message
	}
	writeJSON(w, code, body)
}
