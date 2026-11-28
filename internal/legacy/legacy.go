// Package legacy converts the pose CSVs of the 2026 hackathon build
// (reference_data/cricket/*.csv) into the pose sequence format the browser
// sends (internal/pose), so they can be stored and drawn as references.
//
// What the old files hold (ADR 0008): per row a frame number, a time in
// seconds, and 13 joints with x, y, z and visibility. x and y are MediaPipe
// image coordinates (fractions of the video width and height) with the hip
// midpoint subtracted in every frame. They are not world coordinates in
// metres. z is MediaPipe's rough image depth. The video size is not known.
//
// The conversion therefore:
//   - moves x and y back into the picture, hip midpoint at (0.5, 0.5);
//   - assumes a 16:9 video (1280 x 720);
//   - keeps visibility, drops z, and stores the world values as 0
//     ("not measured");
//   - stores the 20 landmarks the files never had as all 0 ("not measured");
//   - fills dropped frames by linear interpolation at the file's frame rate;
//   - keeps the last 10 s of a longer file (the delivery is at the end).
//
// References made this way are marked origin = legacy_csv and do not count
// toward a shot gate: they cannot be measured like browser references.
package legacy

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"

	"github.com/v1d1tt/talent-bridge/internal/pose"
)

// Joint is one joint of the old files and its MediaPipe landmark index.
type Joint struct {
	Name string // column prefix in the CSV
	Slot int    // index in a MediaPipe Pose Landmarker frame
}

// Joints are the 13 joints of the old files, in column order.
var Joints = [13]Joint{
	{"nose", 0},
	{"left_shoulder", 11}, {"right_shoulder", 12},
	{"left_elbow", 13}, {"right_elbow", 14},
	{"left_wrist", 15}, {"right_wrist", 16},
	{"left_hip", 23}, {"right_hip", 24},
	{"left_knee", 25}, {"right_knee", 26},
	{"left_ankle", 27}, {"right_ankle", 28},
}

// The order of the 4 values of a joint in a Row.
const (
	X = iota // image x minus the hip midpoint's x
	Y
	Z // MediaPipe image depth; read, not stored
	Visibility
)

var fields = [4]string{"x", "y", "z", "visibility"}

// The assumed video size: the old files lost it. 16:9 is the shape of
// broadcast and phone video, and the figures look right at it.
const (
	Width  = 1280
	Height = 720
)

// Row is one row of an old file.
type Row struct {
	T      float64 // seconds from the start
	Joints [len(Joints)][4]float64
}

// Read reads an old CSV file. Columns are found by name, so their order
// does not matter; every joint column must be there.
func Read(r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	head, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("legacy: the file is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("legacy: header: %w", err)
	}
	col := func(name string) (int, error) {
		if i := slices.Index(head, name); i >= 0 {
			return i, nil
		}
		return 0, fmt.Errorf("legacy: column %q is missing", name)
	}
	tCol, err := col("timestamp")
	if err != nil {
		return nil, err
	}
	var cols [len(Joints)][4]int
	for j, joint := range Joints {
		for f, field := range fields {
			if cols[j][f], err = col(joint.Name + "_" + field); err != nil {
				return nil, err
			}
		}
	}

	var rows []Row
	for n := 1; ; n++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("legacy: row %d: %w", n, err)
		}
		var row Row
		if row.T, err = number(rec, tCol); err != nil {
			return nil, fmt.Errorf("legacy: row %d: %w", n, err)
		}
		if len(rows) > 0 && row.T <= rows[len(rows)-1].T {
			return nil, fmt.Errorf("legacy: row %d: time %g is not after the row before", n, row.T)
		}
		for j := range Joints {
			for f := range fields {
				if row.Joints[j][f], err = number(rec, cols[j][f]); err != nil {
					return nil, fmt.Errorf("legacy: row %d: %w", n, err)
				}
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("legacy: the file has no rows")
	}
	return rows, nil
}

func number(rec []string, i int) (float64, error) {
	v, err := strconv.ParseFloat(rec[i], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("value %q is not a number", rec[i])
	}
	return v, nil
}

// Result is a converted file, with what the conversion did to it.
type Result struct {
	Sequence       pose.Sequence
	FilledFrames   int     // dropped frames filled by interpolation
	TrimmedSeconds float64 // cut from the start to fit the 10 s limit
}

// Convert turns the rows of one file into a pose sequence that passes
// pose.Validate.
func Convert(rows []Row) (Result, error) {
	if len(rows) < 2 {
		return Result{}, errors.New("legacy: too short: fewer than 2 rows")
	}
	// The file's frame rate: the usual step between rows (dropped frames
	// make some steps longer).
	steps := make([]float64, len(rows)-1)
	for i := range steps {
		steps[i] = rows[i+1].T - rows[i].T
	}
	slices.Sort(steps)
	fps := math.Round(1 / steps[len(steps)/2])
	if fps < pose.MinFPS || fps > pose.MaxFPS {
		return Result{}, fmt.Errorf("legacy: frame rate %g is outside %d to %d", fps, pose.MinFPS, pose.MaxFPS)
	}

	first, end := rows[0].T, rows[len(rows)-1].T
	n := int(math.Floor((end-first)*fps+1e-6)) + 1
	n = min(n, pose.MaxSeconds*int(fps)) // keep the last 10 s
	if n < pose.MinFrames || float64(n)/fps < pose.MinSeconds {
		return Result{}, fmt.Errorf("legacy: too short: %d frames at %g fps", n, fps)
	}

	res := Result{
		Sequence:       pose.Sequence{FPS: fps, Width: Width, Height: Height, Frames: make([][][]float64, n)},
		TrimmedSeconds: end - float64(n-1)/fps - first,
	}
	if res.TrimmedSeconds < 1e-9 {
		res.TrimmedSeconds = 0
	}
	k := 0 // rows[k] is the last row at or before the frame's time
	for f := range n {
		t := end - float64(n-1-f)/fps
		for k+1 < len(rows) && rows[k+1].T <= t+1e-9 {
			k++
		}
		a, w := rows[k], 0.0
		b := a
		if !near(a.T, t) && k+1 < len(rows) {
			b = rows[k+1]
			w = (t - a.T) / (b.T - a.T)
			res.FilledFrames++
		}
		frame := make([][]float64, pose.Landmarks)
		for l := range frame {
			frame[l] = make([]float64, pose.Values) // not measured: all 0
		}
		for j, joint := range Joints {
			mix := func(i int) float64 { return a.Joints[j][i] + w*(b.Joints[j][i]-a.Joints[j][i]) }
			v := frame[joint.Slot]
			v[pose.X] = mix(X) + 0.5
			v[pose.Y] = mix(Y) + 0.5
			v[pose.Visibility] = mix(Visibility)
		}
		res.Sequence.Frames[f] = frame
	}
	if err := pose.Validate(res.Sequence); err != nil {
		return Result{}, fmt.Errorf("legacy: converted sequence: %w", err)
	}
	return res, nil
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
