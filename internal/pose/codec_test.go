package pose_test

import (
	"math"
	"strings"
	"testing"

	"github.com/v1d1tt/talent-bridge/internal/pose"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := valid()
	b, err := pose.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pose.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if out.FPS != in.FPS || out.Width != in.Width || out.Height != in.Height || len(out.Frames) != len(in.Frames) {
		t.Fatalf("header changed: got fps %v %dx%d %d frames", out.FPS, out.Width, out.Height, len(out.Frames))
	}
	// Values are stored to 1/10000 (0.1 mm for world values), so each one may
	// move by at most half a step.
	for f := range in.Frames {
		for l := range in.Frames[f] {
			for v := range in.Frames[f][l] {
				if d := math.Abs(out.Frames[f][l][v] - in.Frames[f][l][v]); d > 0.5/pose.Scale+1e-12 {
					t.Fatalf("frame %d landmark %d value %d: got %v, want %v", f, l, v, out.Frames[f][l][v], in.Frames[f][l][v])
				}
			}
		}
	}
}

func TestEncodeIsMuchSmallerThanJSON(t *testing.T) {
	s := valid()
	b, err := pose.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	raw := len(s.Frames) * pose.Landmarks * pose.Values * 8 // float64s
	if len(b)*10 > raw {
		t.Errorf("encoded %d bytes for %d raw bytes; want at least 10x smaller", len(b), raw)
	}
	t.Logf("60 frames: %d bytes encoded (raw float64: %d)", len(b), raw)
}

func TestDecodeRejectsDamagedData(t *testing.T) {
	good, err := pose.Encode(valid())
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"empty":      nil,
		"not flate":  []byte("hello, this is not a pose sequence"),
		"truncated":  good[:len(good)/2],
		"wrong data": mustCompress(t, []byte("TBPX0000000000000000")),
	} {
		if _, err := pose.Decode(b); err == nil {
			t.Errorf("%s: Decode succeeded, want an error", name)
		}
	}
}

func mustCompress(t *testing.T, raw []byte) []byte {
	t.Helper()
	b, err := pose.CompressForTest(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A tiny stored value that inflates to gigabytes must stop at the cap.
func TestDecodeStopsAtTheSizeCap(t *testing.T) {
	raw := append([]byte("TBP1"), make([]byte, 30<<20)...) // 30 MB of zeros compresses to ~30 KB
	b := mustCompress(t, raw)
	_, err := pose.Decode(b)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Decode = %v, want a 'too large' error", err)
	}
}
