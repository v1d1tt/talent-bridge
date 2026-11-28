package pose

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Scale is the fixed-point step of the stored form: every value is kept as
// a whole number of 1/10000 units (0.1 mm for world values, 0.1 px on a
// 1000 px wide video). That is far finer than MediaPipe's own noise.
const Scale = 10000

// Stored form, version 1 ("TBP1"). Before compression:
//
//	magic   "TBP1"          4 bytes
//	fps     float32         4 bytes, little endian
//	width   uint16          2 bytes
//	height  uint16          2 bytes
//	frames  uint16          2 bytes
//	values  frames x 33 x 6 signed varints (zigzag)
//
// Each value is round(v * Scale) minus the same value in the frame before
// (the first frame is stored as is). Bodies move little between frames, so
// most differences fit in one byte. The whole thing is then compressed with
// DEFLATE (RFC 1951, Go's compress/flate).
var magic = [4]byte{'T', 'B', 'P', '1'}

const headerSize = 14

// Encode returns the compact stored form of a sequence that passed Validate.
func Encode(s Sequence) ([]byte, error) {
	if len(s.Frames) > math.MaxUint16 || s.Width > math.MaxUint16 || s.Height > math.MaxUint16 {
		return nil, errors.New("pose: sequence too large to encode")
	}
	raw := make([]byte, 0, headerSize+len(s.Frames)*Landmarks*Values*2)
	raw = append(raw, magic[:]...)
	raw = binary.LittleEndian.AppendUint32(raw, math.Float32bits(float32(s.FPS)))
	raw = binary.LittleEndian.AppendUint16(raw, uint16(s.Width))
	raw = binary.LittleEndian.AppendUint16(raw, uint16(s.Height))
	raw = binary.LittleEndian.AppendUint16(raw, uint16(len(s.Frames)))

	var prev [Landmarks * Values]int64
	for f, frame := range s.Frames {
		if len(frame) != Landmarks {
			return nil, fmt.Errorf("pose: frame %d has %d landmarks", f, len(frame))
		}
		for l, v := range frame {
			if len(v) != Values {
				return nil, fmt.Errorf("pose: frame %d landmark %d has %d values", f, l, len(v))
			}
			for i, x := range v {
				q := int64(math.Round(x * Scale))
				k := l*Values + i
				raw = binary.AppendVarint(raw, q-prev[k])
				prev[k] = q
			}
		}
	}
	return compress(raw)
}

// Decode reads the stored form back. Values come back rounded to 1/Scale.
func Decode(b []byte) (Sequence, error) {
	raw, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(b)), maxRaw+1))
	if err != nil {
		return Sequence{}, fmt.Errorf("pose: decompress: %w", err)
	}
	if len(raw) > maxRaw {
		return Sequence{}, errors.New("pose: stored sequence is too large")
	}
	if len(raw) < headerSize || !bytes.Equal(raw[:4], magic[:]) {
		return Sequence{}, errors.New("pose: not a TBP1 pose sequence")
	}
	s := Sequence{
		FPS:    float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[4:]))),
		Width:  int(binary.LittleEndian.Uint16(raw[8:])),
		Height: int(binary.LittleEndian.Uint16(raw[10:])),
	}
	n := int(binary.LittleEndian.Uint16(raw[12:]))
	rest := raw[headerSize:]

	var prev [Landmarks * Values]int64
	s.Frames = make([][][]float64, n)
	for f := range n {
		frame := make([][]float64, Landmarks)
		for l := range frame {
			v := make([]float64, Values)
			for i := range v {
				d, size := binary.Varint(rest)
				if size <= 0 {
					return Sequence{}, fmt.Errorf("pose: data ends early in frame %d", f)
				}
				rest = rest[size:]
				k := l*Values + i
				prev[k] += d
				v[i] = float64(prev[k]) / Scale
			}
			frame[l] = v
		}
		s.Frames[f] = frame
	}
	if len(rest) != 0 {
		return Sequence{}, fmt.Errorf("pose: %d bytes left after the last frame", len(rest))
	}
	return s, nil
}

// maxRaw caps decompression (a zip bomb guard): 600 frames x 198 values x
// at most 10 bytes per varint, plus the header.
const maxRaw = headerSize + MaxFrames*Landmarks*Values*binary.MaxVarintLen64

func compress(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
