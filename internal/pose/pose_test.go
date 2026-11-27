package pose_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/pose"
)

// valid returns a pose sequence that passes every check: 2 s at 30 fps,
// a person standing still with every landmark visible.
func valid() pose.Sequence {
	s := pose.Sequence{FPS: 30, Width: 1280, Height: 720}
	for f := range 60 {
		frame := make([][]float64, pose.Landmarks)
		for l := range frame {
			t := float64(f) / 60
			frame[l] = []float64{0.4 + 0.01*float64(l%5) + 0.05*t, 0.2 + 0.02*float64(l), 0.9, 0.1 * t, -0.5 + 0.03*float64(l), 0.05}
		}
		s.Frames = append(s.Frames, frame)
	}
	return s
}

func TestValidateAcceptsAGoodSequence(t *testing.T) {
	if err := pose.Validate(valid()); err != nil {
		t.Fatalf("Validate(valid) = %v, want nil", err)
	}
}

// Every reason the API can give for a 422, one row each.
func TestValidateRejectsBadShapes(t *testing.T) {
	frames := func(n int) func(*pose.Sequence) {
		return func(s *pose.Sequence) {
			for len(s.Frames) < n {
				s.Frames = append(s.Frames, s.Frames[len(s.Frames)-1])
			}
			s.Frames = s.Frames[:n]
		}
	}
	set := func(frame, landmark, field int, v float64) func(*pose.Sequence) {
		return func(s *pose.Sequence) {
			row := append([]float64(nil), s.Frames[frame][landmark]...)
			row[field] = v
			fr := append([][]float64(nil), s.Frames[frame]...)
			fr[landmark] = row
			s.Frames[frame] = fr
		}
	}
	hide := func(from, to int) func(*pose.Sequence) { // visibility 0 for frames [from, to)
		return func(s *pose.Sequence) {
			for f := from; f < to; f++ {
				fr := make([][]float64, pose.Landmarks)
				for l := range fr {
					fr[l] = []float64{0, 0, 0, 0, 0, 0}
				}
				s.Frames[f] = fr
			}
		}
	}

	tests := []struct {
		name   string
		change func(*pose.Sequence)
		reason string
		inMsg  string // the message must name the problem
	}{
		{"no frames", func(s *pose.Sequence) { s.Frames = nil }, "too_few_frames", "0 frames"},
		{"14 frames", frames(14), "too_few_frames", "at least 15"},
		{"601 frames", func(s *pose.Sequence) { s.FPS = 60; frames(601)(s) }, "too_many_frames", "at most 600"},
		{"fps 14", func(s *pose.Sequence) { s.FPS = 14 }, "fps_out_of_range", "15 to 60"},
		{"fps 61", func(s *pose.Sequence) { s.FPS = 61 }, "fps_out_of_range", "15 to 60"},
		{"fps missing", func(s *pose.Sequence) { s.FPS = 0 }, "fps_out_of_range", "15 to 60"},
		{"fps NaN", func(s *pose.Sequence) { s.FPS = math.NaN() }, "fps_out_of_range", "15 to 60"},
		{"shorter than 1 s", func(s *pose.Sequence) { s.FPS = 60; frames(50)(s) }, "clip_too_short", "at least 1 s"},
		{"longer than 10 s", func(s *pose.Sequence) { s.FPS = 15; frames(151)(s) }, "clip_too_long", "at most 10 s"},
		{"width missing", func(s *pose.Sequence) { s.Width = 0 }, "bad_video_size", "16 to 7680"},
		{"height too big", func(s *pose.Sequence) { s.Height = 9000 }, "bad_video_size", "16 to 7680"},
		{"32 landmarks", func(s *pose.Sequence) { s.Frames[3] = s.Frames[3][:32] }, "wrong_landmark_count", "frame 3 has 32"},
		{"34 landmarks", func(s *pose.Sequence) { s.Frames[0] = append(append([][]float64(nil), s.Frames[0]...), s.Frames[0][0]) }, "wrong_landmark_count", "frame 0 has 34"},
		{"5 values", func(s *pose.Sequence) {
			fr := append([][]float64(nil), s.Frames[2]...)
			fr[7] = fr[7][:5]
			s.Frames[2] = fr
		}, "wrong_value_count", "frame 2, landmark 7 has 5"},
		{"x too far left", set(4, 11, 0, -0.51), "value_out_of_range", "frame 4, landmark 11: x"},
		{"y too far down", set(4, 11, 1, 1.51), "value_out_of_range", "landmark 11: y"},
		{"visibility above 1", set(5, 0, 2, 1.01), "value_out_of_range", "visibility"},
		{"visibility below 0", set(5, 0, 2, -0.01), "value_out_of_range", "visibility"},
		{"world x 3.01 m", set(6, 15, 3, 3.01), "value_out_of_range", "world_x"},
		{"world y -3.01 m", set(6, 15, 4, -3.01), "value_out_of_range", "world_y"},
		{"world z NaN", set(6, 15, 5, math.NaN()), "value_out_of_range", "world_z"},
		{"world z +Inf", set(6, 15, 5, math.Inf(1)), "value_out_of_range", "world_z"},
		{"no person in most frames", hide(0, 31), "person_not_found", "31 of 60 frames"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := valid()
			tc.change(&s)
			err := pose.Validate(s)
			var ve *pose.Error
			if !errors.As(err, &ve) {
				t.Fatalf("Validate = %v, want a *pose.Error", err)
			}
			if ve.Reason != tc.reason {
				t.Errorf("reason = %q, want %q (message %q)", ve.Reason, tc.reason, ve.Message)
			}
			if !strings.Contains(ve.Message, tc.inMsg) {
				t.Errorf("message %q does not contain %q", ve.Message, tc.inMsg)
			}
		})
	}
}

func TestValidateAllowsSomeFramesWithNoPerson(t *testing.T) {
	s := valid()
	for f := range 30 { // exactly half: still accepted
		fr := make([][]float64, pose.Landmarks)
		for l := range fr {
			fr[l] = []float64{0, 0, 0, 0, 0, 0}
		}
		s.Frames[f] = fr
	}
	if err := pose.Validate(s); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}
