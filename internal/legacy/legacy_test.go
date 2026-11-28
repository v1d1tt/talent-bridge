package legacy_test

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/legacy"
	"github.com/v1d1tt/talent-bridge/internal/pose"
)

// testdata/sachin-first-22.csv is the first 22 rows of the old
// Sachin_Straightdrive_Shot.csv, unchanged: frames 0-22 at 15 fps, and
// frame 18 is missing (a dropped frame between t=1.1333 and t=1.2667).

func readExcerpt(t *testing.T) []legacy.Row {
	t.Helper()
	f, err := os.Open("testdata/sachin-first-22.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := legacy.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestJointSlots(t *testing.T) {
	// The CSV joint names and their MediaPipe Pose Landmarker indexes.
	want := map[string]int{
		"nose": 0, "left_shoulder": 11, "right_shoulder": 12, "left_elbow": 13, "right_elbow": 14,
		"left_wrist": 15, "right_wrist": 16, "left_hip": 23, "right_hip": 24, "left_knee": 25,
		"right_knee": 26, "left_ankle": 27, "right_ankle": 28,
	}
	if len(legacy.Joints) != len(want) {
		t.Fatalf("%d joints, want %d", len(legacy.Joints), len(want))
	}
	for _, j := range legacy.Joints {
		if slot, ok := want[j.Name]; !ok || slot != j.Slot {
			t.Errorf("joint %q -> slot %d, want %d (known: %v)", j.Name, j.Slot, slot, ok)
		}
	}
}

func TestReadRealRows(t *testing.T) {
	rows := readExcerpt(t)
	if len(rows) != 22 {
		t.Fatalf("read %d rows, want 22", len(rows))
	}
	// Values straight from the file's first data row.
	first := rows[0]
	if !near(first.T, 0) || !near(rows[21].T, 22.0/15) {
		t.Errorf("times %v .. %v, want 0 .. 22/15", first.T, rows[21].T)
	}
	if !near(first.Joints[0][legacy.X], -0.07811000943183899) || !near(first.Joints[0][legacy.Y], -0.0779067724943161) {
		t.Errorf("nose = %v", first.Joints[0])
	}
	if !near(first.Joints[0][legacy.Visibility], 0.951804518699646) {
		t.Errorf("nose visibility = %v", first.Joints[0][legacy.Visibility])
	}
	// Every row is centred on the hips: the two hip points mirror each other.
	for i, r := range rows {
		lh, rh := r.Joints[7], r.Joints[8]
		if !near(lh[legacy.X], -rh[legacy.X]) || !near(lh[legacy.Y], -rh[legacy.Y]) {
			t.Errorf("row %d: hips %v and %v are not centred on 0", i, lh, rh)
		}
	}
}

// header is the real CSV header, for the error cases.
func header() string {
	cols := []string{"frame", "timestamp"}
	for _, j := range legacy.Joints {
		for _, f := range []string{"x", "y", "z", "visibility"} {
			cols = append(cols, j.Name+"_"+f)
		}
	}
	return strings.Join(cols, ",")
}

func row(frame int, t float64, v float64) string {
	cols := []string{fmt.Sprint(frame), fmt.Sprint(t)}
	for range legacy.Joints {
		cols = append(cols, fmt.Sprint(v), fmt.Sprint(-v), "0.1", "0.9")
	}
	return strings.Join(cols, ",")
}

func TestReadRefusesBadFiles(t *testing.T) {
	h := header()
	for _, tc := range []struct {
		name, csv, want string
	}{
		{"empty", "", "empty"},
		{"no rows", h + "\n", "no rows"},
		{"missing column", strings.Replace(h, ",left_knee_y", "", 1) + "\n" + row(0, 0, 0.1), `column "left_knee_y"`},
		{"not a number", h + "\n" + strings.Replace(row(0, 0, 0.1), "0.9", "high", 1), "row 1"},
		{"NaN", h + "\n" + strings.Replace(row(0, 0, 0.1), "0.9", "NaN", 1), "row 1"},
		{"time goes back", h + "\n" + row(0, 0.5, 0.1) + "\n" + row(1, 0.4, 0.1), "row 2"},
		{"short row", h + "\n" + "0,0,0.1", "row 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := legacy.Read(strings.NewReader(tc.csv))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one that mentions %q", err, tc.want)
			}
		})
	}
}

func TestConvertRealRows(t *testing.T) {
	rows := readExcerpt(t)
	res, err := legacy.Convert(rows)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sequence
	if s.FPS != 15 || s.Width != legacy.Width || s.Height != legacy.Height {
		t.Errorf("fps %v, size %dx%d", s.FPS, s.Width, s.Height)
	}
	// Frames 0-22 at 15 fps, with the dropped frame filled in.
	if len(s.Frames) != 23 || res.FilledFrames != 1 || res.TrimmedSeconds != 0 {
		t.Errorf("%d frames, %d filled, %.2f s trimmed; want 23, 1, 0", len(s.Frames), res.FilledFrames, res.TrimmedSeconds)
	}
	if err := pose.Validate(s); err != nil {
		t.Fatalf("converted sequence fails the browser's checks: %v", err)
	}

	// A known joint: image x/y are the hip-centred values moved to the middle
	// of the picture; visibility is kept; world values are not measured (0).
	nose := s.Frames[0][0]
	want := []float64{-0.07811000943183899 + 0.5, -0.0779067724943161 + 0.5, 0.951804518699646, 0, 0, 0}
	for i := range want {
		if !near(nose[i], want[i]) {
			t.Errorf("frame 0 nose = %v, want %v", nose, want)
			break
		}
	}
	// The hip midpoint sits in the middle of the picture in every frame.
	for f, fr := range s.Frames {
		mx, my := (fr[23][pose.X]+fr[24][pose.X])/2, (fr[23][pose.Y]+fr[24][pose.Y])/2
		if !near(mx, 0.5) || !near(my, 0.5) {
			t.Errorf("frame %d: hip midpoint at (%v, %v), want (0.5, 0.5)", f, mx, my)
		}
	}
	// Landmarks the old files never had (eyes, ears, hands, heels, feet) are
	// "not measured": all six values 0.
	for _, slot := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 17, 18, 19, 20, 21, 22, 29, 30, 31, 32} {
		for _, v := range s.Frames[5][slot] {
			if v != 0 {
				t.Fatalf("slot %d = %v, want all 0", slot, s.Frames[5][slot])
			}
		}
	}
	// Frame 18 (t = 1.2 s) was dropped: it is the midpoint of frames 17 and 19.
	a, b := rows[17].Joints[0], rows[18].Joints[0] // rows[18] is frame 19
	got := s.Frames[18][0]
	if !near(got[pose.X], (a[legacy.X]+b[legacy.X])/2+0.5) || !near(got[pose.Visibility], (a[legacy.Visibility]+b[legacy.Visibility])/2) {
		t.Errorf("filled frame 18 nose = %v, want the midpoint of %v and %v", got, a, b)
	}
	// Frames the file has come through as they are.
	if !near(s.Frames[19][0][pose.X], rows[18].Joints[0][legacy.X]+0.5) {
		t.Errorf("frame 19 nose x = %v, want %v", s.Frames[19][0][pose.X], rows[18].Joints[0][legacy.X]+0.5)
	}

	// It survives the stored form (TBP1) like a browser sequence.
	enc, err := pose.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	if back, err := pose.Decode(enc); err != nil || len(back.Frames) != 23 {
		t.Fatalf("round trip: %v, %d frames", err, len(back.Frames))
	}
}

func synthetic(n int, fps float64) []legacy.Row {
	rows := make([]legacy.Row, n)
	for i := range rows {
		rows[i].T = float64(i) / fps
		for j := range rows[i].Joints {
			rows[i].Joints[j] = [4]float64{0.001 * float64(i), -0.1, 0.2, 0.9}
		}
	}
	return rows
}

func TestConvertKeepsTheLast10Seconds(t *testing.T) {
	// Bhuvneshwar's file is 14.8 s at 30 fps; the delivery is at the end.
	rows := synthetic(446, 30)
	res, err := legacy.Convert(rows)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sequence
	if len(s.Frames) != 300 || s.FPS != 30 {
		t.Fatalf("%d frames at %v fps, want 300 at 30", len(s.Frames), s.FPS)
	}
	if !near(res.TrimmedSeconds, 445.0/30-299.0/30) {
		t.Errorf("trimmed %.3f s", res.TrimmedSeconds)
	}
	last := s.Frames[len(s.Frames)-1][0][pose.X]
	if !near(last, 0.445+0.5) {
		t.Errorf("last frame x = %v, want the file's last row (%v)", last, 0.445+0.5)
	}
	if err := pose.Validate(s); err != nil {
		t.Fatal(err)
	}
}

func TestConvertFrameRates(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []legacy.Row
		fps  float64
		err  string
	}{
		{"15 fps", synthetic(40, 15), 15, ""},
		{"25 fps (Shami)", synthetic(240, 25), 25, ""},
		{"30 fps", synthetic(60, 30), 30, ""},
		{"too slow", synthetic(40, 10), 0, "frame rate"},
		{"too short", synthetic(10, 15), 0, "too short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := legacy.Convert(tc.rows)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Sequence.FPS != tc.fps {
				t.Errorf("fps = %v, want %v", res.Sequence.FPS, tc.fps)
			}
		})
	}
}
