// Package analysis saves and serves analyses (sessions): the pose sequence
// the browser made from a clip, and its 2-3 key frames in Blob Storage.
// Scoring (slice 5) fills in the rest later.
//
//	POST /api/analyses                         athlete: check + store the pose sequence, get upload URLs
//	POST /api/analyses/{id}/key-frames/check   owner: check what the browser uploaded
//	GET  /api/analyses/{id}                    owner: the analysis, with read URLs for the key frames
package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/auth"
	"github.com/v1d1tt/talent-bridge/internal/blob"
	"github.com/v1d1tt/talent-bridge/internal/database"
	"github.com/v1d1tt/talent-bridge/internal/pose"
	"github.com/v1d1tt/talent-bridge/internal/store"
)

// Limits on what one request and one athlete may do.
const (
	MaxBodyBytes      = 4 << 20   // 600 frames x 33 x 6 numbers is about 1.5 MB as JSON
	MinKeyFrames      = 2         // PRD: 2-3 key frames per session
	MaxKeyFrames      = 3         //
	MaxKeyFrameBytes  = 512 << 10 // a 640 px JPEG is about 30-80 KB
	MinKeyFrameBytes  = 100       // a smaller "JPEG" is not a picture
	DefaultDailyLimit = 30        // analyses per athlete per 24 hours (Neon has 1 GB)
)

// Service holds the database and the key frame store. A nil store means
// key frames are off; analyses are still saved.
type Service struct {
	db    *database.DB
	blobs *blob.Store

	// DailyLimit is how many analyses one athlete may save per 24 hours.
	DailyLimit int
}

// New returns the service. blobs may be nil (no Blob Storage configured).
func New(db *database.DB, blobs *blob.Store) *Service {
	return &Service{db: db, blobs: blobs, DailyLimit: DefaultDailyLimit}
}

// Disabled answers 503 on every route: the server has no database.
func Disabled() *Service { return &Service{} }

// KeyFramesOn reports whether Blob Storage is configured.
func (s *Service) KeyFramesOn() bool { return s.blobs != nil }

// createRequest is the body of POST /api/analyses. Unknown fields are
// refused, so nothing else (a video, an image) can ride along.
type createRequest struct {
	PoseSequence pose.Sequence `json:"pose_sequence"`
	KeyFrames    []struct {
		FrameIndex int `json:"frame_index"`
	} `json:"key_frames"`
}

type uploadJSON struct {
	FrameIndex int       `json:"frame_index"`
	UploadURL  string    `json:"upload_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Create serves POST /api/analyses. Guard it with Roles(athlete).
func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return
	}
	sess, _ := auth.SessionFrom(r.Context())
	uid, ok := parseUUID(sess.User.ID)
	if !ok {
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
	if err := pose.Validate(req.PoseSequence); err != nil {
		var pe *pose.Error
		if errors.As(err, &pe) {
			writeError(w, http.StatusUnprocessableEntity, pe.Reason, pe.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}

	var indexes []int
	if s.blobs != nil { // with no storage the key frames are ignored, not refused
		n := len(req.PoseSequence.Frames)
		if k := len(req.KeyFrames); k < MinKeyFrames || k > MaxKeyFrames {
			writeError(w, http.StatusUnprocessableEntity, "key_frame_count", fmt.Sprintf("Send %d to %d key frames, not %d.", MinKeyFrames, MaxKeyFrames, k))
			return
		}
		for _, k := range req.KeyFrames {
			if k.FrameIndex < 0 || k.FrameIndex >= n || slices.Contains(indexes, k.FrameIndex) {
				writeError(w, http.StatusUnprocessableEntity, "key_frame_index", fmt.Sprintf("Key frame %d is not a new frame of this clip (0 to %d).", k.FrameIndex, n-1))
				return
			}
			indexes = append(indexes, k.FrameIndex)
		}
		slices.Sort(indexes)
	}

	since := pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true}
	count, err := s.db.Queries.CountAnalysesSince(r.Context(), store.CountAnalysesSinceParams{UserID: uid, CreatedAt: since})
	if err != nil {
		s.internal(w, r, "count analyses", err)
		return
	}
	if count >= int64(s.DailyLimit) {
		writeError(w, http.StatusTooManyRequests, "daily_limit", fmt.Sprintf("You can save %d analyses per day. Try again tomorrow.", s.DailyLimit))
		return
	}

	encoded, err := pose.Encode(req.PoseSequence)
	if err != nil {
		s.internal(w, r, "encode pose sequence", err)
		return
	}
	var id pgtype.UUID
	var paths []string
	err = s.db.Tx(r.Context(), func(q *store.Queries) error {
		row, err := q.CreateAnalysis(r.Context(), store.CreateAnalysisParams{
			UserID:       uid,
			Fps:          float32(req.PoseSequence.FPS),
			FrameCount:   int32(len(req.PoseSequence.Frames)),
			PoseSequence: encoded,
		})
		if err != nil {
			return err
		}
		id = row.ID
		for _, i := range indexes {
			// The path holds only random ids: no user id, no name.
			p := fmt.Sprintf("analyses/%s/%d.jpg", id.String(), i)
			if _, err := q.CreateKeyFrame(r.Context(), store.CreateKeyFrameParams{AnalysisID: id, FrameIndex: int32(i), ObjectPath: p}); err != nil {
				return err
			}
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		s.internal(w, r, "create analysis", err)
		return
	}

	uploads := make([]uploadJSON, 0, len(paths))
	for i, p := range paths {
		u, expires, err := s.blobs.UploadURL(r.Context(), p)
		if err != nil {
			s.internal(w, r, "sign upload URL", err)
			return
		}
		uploads = append(uploads, uploadJSON{FrameIndex: indexes[i], UploadURL: u, ExpiresAt: expires.UTC()})
	}
	slog.InfoContext(r.Context(), "analysis: created", "user", sess.User.ID, "analysis", id.String(),
		"frames", len(req.PoseSequence.Frames), "stored_bytes", len(encoded), "key_frames", len(uploads))
	writeJSON(w, http.StatusCreated, map[string]any{"id": id.String(), "key_frames": uploads})
}

// CheckKeyFrames serves POST /api/analyses/{id}/key-frames/check. The
// browser calls it after its uploads. Each pending key frame is checked in
// Blob Storage: a JPEG of a sane size becomes uploaded; anything else is
// deleted and marked rejected; a missing one stays pending. The upload URL
// may only create, so a checked blob cannot be replaced afterwards. The
// sweep (Sweep) settles whatever is left once the upload URLs expire.
func (s *Service) CheckKeyFrames(w http.ResponseWriter, r *http.Request) {
	a, ok := s.ownAnalysis(w, r)
	if !ok {
		return
	}
	frames, err := s.db.Queries.ListKeyFrames(r.Context(), a.ID)
	if err != nil {
		s.internal(w, r, "list key frames", err)
		return
	}
	for _, k := range frames {
		if k.State != store.KeyFrameStatePending || s.blobs == nil {
			continue
		}
		state, size, err := s.judge(r.Context(), k)
		if errors.Is(err, blob.ErrNotFound) {
			continue // not uploaded (yet)
		}
		if err != nil {
			s.internal(w, r, "check key frame", err)
			return
		}
		if err := s.db.Queries.SetKeyFrameState(r.Context(), store.SetKeyFrameStateParams{ID: k.ID, State: state, ByteSize: size}); err != nil {
			s.internal(w, r, "set key frame state", err)
			return
		}
	}
	s.writeAnalysis(w, r, a)
}

// judge looks at a key frame's blob: a JPEG of a sane size is uploaded;
// anything else is deleted and rejected. A missing blob is ErrNotFound.
func (s *Service) judge(ctx context.Context, k store.KeyFrame) (store.KeyFrameState, pgtype.Int4, error) {
	info, err := s.blobs.Inspect(ctx, k.ObjectPath)
	if err != nil {
		return "", pgtype.Int4{}, err
	}
	size := pgtype.Int4{Int32: int32(min(info.Size, math.MaxInt32)), Valid: true}
	if info.Size >= MinKeyFrameBytes && info.Size <= MaxKeyFrameBytes && bytes.HasPrefix(info.Head, []byte{0xFF, 0xD8, 0xFF}) {
		return store.KeyFrameStateUploaded, size, nil
	}
	slog.WarnContext(ctx, "analysis: key frame rejected", "analysis", k.AnalysisID.String(), "frame", k.FrameIndex, "bytes", info.Size)
	return store.KeyFrameStateRejected, size, s.blobs.Delete(ctx, k.ObjectPath)
}

// Sweep settles, once, every key frame that is not uploaded and whose upload
// window closed before `now`: no upload URL for it works any more, so its
// state is final. A good JPEG nobody checked becomes uploaded; anything else
// (missing, junk, or junk uploaded again after a rejection) is deleted and
// rejected. It returns how many key frames it settled.
func (s *Service) Sweep(ctx context.Context, now time.Time) (int, error) {
	if s.db == nil || s.blobs == nil {
		return 0, nil
	}
	closed := pgtype.Timestamptz{Time: now.Add(-blob.UploadTTL - time.Minute), Valid: true} // a minute for clock skew
	frames, err := s.db.Queries.ListUnsettledKeyFrames(ctx, closed)
	if err != nil {
		return 0, err
	}
	for _, k := range frames {
		state, size, err := store.KeyFrameStateRejected, pgtype.Int4{}, error(nil)
		if k.State == store.KeyFrameStatePending {
			state, size, err = s.judge(ctx, k)
		} else {
			err = s.blobs.Delete(ctx, k.ObjectPath) // rejected earlier: remove anything uploaded since
		}
		if errors.Is(err, blob.ErrNotFound) {
			state, err = store.KeyFrameStateRejected, nil
		}
		if err != nil {
			return 0, err
		}
		if err := s.db.Queries.SettleKeyFrame(ctx, store.SettleKeyFrameParams{ID: k.ID, State: state, ByteSize: size}); err != nil {
			return 0, err
		}
	}
	return len(frames), nil
}

// RunSweeper calls Sweep every `every` until ctx ends. Each replica runs
// one; two sweeps of the same key frame are harmless (the update is
// conditional and a delete of a missing blob is not an error).
func (s *Service) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := s.Sweep(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "analysis: key frame sweep", "err", err)
		} else if n > 0 {
			slog.InfoContext(ctx, "analysis: key frames settled", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Get serves GET /api/analyses/{id}. Guard it with Roles(athlete); only the
// owner finds the analysis, everyone else gets 404.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.ownAnalysis(w, r); ok {
		s.writeAnalysis(w, r, a)
	}
}

// ownAnalysis loads the analysis in the path if the signed-in user owns it.
// A missing, malformed or someone else's id answers 404, so ids of other
// people's analyses cannot be probed.
func (s *Service) ownAnalysis(w http.ResponseWriter, r *http.Request) (store.GetOwnAnalysisRow, bool) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "")
		return store.GetOwnAnalysisRow{}, false
	}
	sess, _ := auth.SessionFrom(r.Context())
	id, ok1 := parseUUID(r.PathValue("id"))
	uid, ok2 := parseUUID(sess.User.ID)
	if !ok1 || !ok2 {
		writeError(w, http.StatusNotFound, "not_found", "")
		return store.GetOwnAnalysisRow{}, false
	}
	a, err := s.db.Queries.GetOwnAnalysis(r.Context(), store.GetOwnAnalysisParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "")
		return a, false
	}
	if err != nil {
		s.internal(w, r, "get analysis", err)
		return a, false
	}
	return a, true
}

type keyFrameJSON struct {
	FrameIndex int    `json:"frame_index"`
	State      string `json:"state"`         // pending | uploaded | rejected
	URL        string `json:"url,omitempty"` // read URL, only when uploaded
}

type analysisJSON struct {
	ID           string         `json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	FPS          float64        `json:"fps"`
	FrameCount   int            `json:"frame_count"`
	Duration     float64        `json:"duration_s"`
	PoseSequence pose.Sequence  `json:"pose_sequence"`
	Score        *int           `json:"score"` // null until scoring (slice 5)
	KeyFramesOn  bool           `json:"key_frames_on"`
	KeyFrames    []keyFrameJSON `json:"key_frames"`
}

func (s *Service) writeAnalysis(w http.ResponseWriter, r *http.Request, a store.GetOwnAnalysisRow) {
	seq, err := pose.Decode(a.PoseSequence)
	if err != nil {
		s.internal(w, r, "decode pose sequence", err)
		return
	}
	frames, err := s.db.Queries.ListKeyFrames(r.Context(), a.ID)
	if err != nil {
		s.internal(w, r, "list key frames", err)
		return
	}
	out := analysisJSON{
		ID:           a.ID.String(),
		CreatedAt:    a.CreatedAt.Time.UTC(),
		FPS:          seq.FPS,
		FrameCount:   int(a.FrameCount),
		Duration:     math.Round(seq.Duration()*100) / 100,
		PoseSequence: seq,
		KeyFramesOn:  s.blobs != nil,
		KeyFrames:    []keyFrameJSON{},
	}
	for _, k := range frames {
		kj := keyFrameJSON{FrameIndex: int(k.FrameIndex), State: string(k.State)}
		if k.State == store.KeyFrameStateUploaded && s.blobs != nil {
			if kj.URL, err = s.blobs.ReadURL(r.Context(), k.ObjectPath); err != nil {
				s.internal(w, r, "sign read URL", err)
				return
			}
		}
		out.KeyFrames = append(out.KeyFrames, kj)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	if errors.Is(err, context.Canceled) {
		return // the browser went away
	}
	slog.ErrorContext(r.Context(), "analysis: "+what, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "")
}

func parseUUID(s string) (pgtype.UUID, bool) {
	var u pgtype.UUID
	if len(s) != 36 || u.Scan(s) != nil {
		return u, false
	}
	return u, true
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
