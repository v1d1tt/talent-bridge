// Package pose holds the pose sequence that the browser sends (ADR 0001):
// its shape checks and its compact stored form.
//
// The server never trusts the browser: every sequence is checked here before
// it is stored, and a bad one gets a 422 with a reason code and a plain
// English message.
package pose

import "fmt"

// Landmarks is the number of body points MediaPipe Pose Landmarker returns
// per frame. Values is the number of values per landmark.
const (
	Landmarks = 33
	Values    = 6
)

// The order of the values of one landmark.
const (
	X          = iota // image x, 0..1 of the width (a little outside when off screen)
	Y                 // image y, 0..1 of the height
	Visibility        // 0..1
	WorldX            // metres from the midpoint of the hips
	WorldY
	WorldZ
)

var fieldNames = [Values]string{"x", "y", "visibility", "world_x", "world_y", "world_z"}

// Limits from the PRD (section 4): clips of 1-10 s, 15-60 fps, at most 600 frames.
const (
	MinFrames     = 15
	MaxFrames     = 600
	MinFPS        = 15
	MaxFPS        = 60
	MinSeconds    = 1
	MaxSeconds    = 10
	MinVideoSide  = 16
	MaxVideoSide  = 7680
	maxImageCoord = 1.5  // MediaPipe guesses landmarks a little outside the image
	minImageCoord = -0.5 //
	maxWorld      = 3.0  // metres from the hips: more than any human reach
)

// Sequence is one clip's landmarks in time order, plus the frame rate and
// the video size (normalised x and y need the aspect ratio to be drawn or
// measured). Frames[f][l] holds the 6 values of landmark l in frame f.
type Sequence struct {
	FPS    float64       `json:"fps"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Frames [][][]float64 `json:"frames"`
}

// Error is a pose sequence the server refuses. Reason is a stable code for
// the page; Message is plain English for the athlete.
type Error struct {
	Reason  string
	Message string
}

func (e *Error) Error() string { return e.Reason + ": " + e.Message }

func fail(reason, format string, a ...any) *Error {
	return &Error{Reason: reason, Message: fmt.Sprintf(format, a...)}
}

// Validate checks the shape and the ranges of a pose sequence. It returns a
// *Error for the first problem it finds, or nil.
func Validate(s Sequence) error {
	n := len(s.Frames)
	switch {
	case n < MinFrames:
		return fail("too_few_frames", "The clip has %d frames. It needs at least %d.", n, MinFrames)
	case n > MaxFrames:
		return fail("too_many_frames", "The clip has %d frames. It can have at most %d.", n, MaxFrames)
	case !(s.FPS >= MinFPS && s.FPS <= MaxFPS): // also catches NaN
		return fail("fps_out_of_range", "The frame rate must be %d to %d frames per second.", MinFPS, MaxFPS)
	}
	if secs := float64(n) / s.FPS; secs < MinSeconds {
		return fail("clip_too_short", "The clip is %.1f s long. It must be at least %d s.", secs, MinSeconds)
	} else if secs > MaxSeconds {
		return fail("clip_too_long", "The clip is %.1f s long. It must be at most %d s.", secs, MaxSeconds)
	}
	if !side(s.Width) || !side(s.Height) {
		return fail("bad_video_size", "The video size must be %d to %d pixels on each side.", MinVideoSide, MaxVideoSide)
	}

	empty := 0
	for f, frame := range s.Frames {
		if len(frame) != Landmarks {
			return fail("wrong_landmark_count", "Each frame needs %d landmarks, but frame %d has %d.", Landmarks, f, len(frame))
		}
		seen := false
		for l, v := range frame {
			if len(v) != Values {
				return fail("wrong_value_count", "Each landmark needs %d values, but frame %d, landmark %d has %d.", Values, f, l, len(v))
			}
			for i, x := range v {
				if lo, hi := limits(i); !(x >= lo && x <= hi) { // also catches NaN
					return fail("value_out_of_range", "In frame %d, landmark %d: %s is %g, which is outside %g to %g.", f, l, fieldNames[i], x, lo, hi)
				}
			}
			if v[Visibility] > 0 {
				seen = true
			}
		}
		if !seen {
			empty++
		}
	}
	// A frame with no person in it is sent with visibility 0 everywhere. A few
	// are normal (motion blur); most means the wrong clip.
	if empty*2 > n {
		return fail("person_not_found", "No person was found in %d of %d frames. Film one person, head to feet, in good light.", empty, n)
	}
	return nil
}

func side(px int) bool { return px >= MinVideoSide && px <= MaxVideoSide }

func limits(field int) (lo, hi float64) {
	switch field {
	case X, Y:
		return minImageCoord, maxImageCoord
	case Visibility:
		return 0, 1
	default:
		return -maxWorld, maxWorld
	}
}

// Duration is the clip length in seconds.
func (s Sequence) Duration() float64 {
	if s.FPS <= 0 {
		return 0
	}
	return float64(len(s.Frames)) / s.FPS
}
