package pcm

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// FuzzDecode decodes arbitrary bytes in every format at both widths and checks
// each sample against the reference and the full-scale range.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{0x00, 0x80, 0xff, 0x7f, 0x01})
	f.Add(bytes.Repeat([]byte{0x80, 0x00}, 40))
	f.Fuzz(func(t *testing.T, src []byte) {
		for _, fm := range Formats() {
			got := make([]float32, len(src))
			n := Decode(got, src, fm)
			if n != len(src)/fm.Width() {
				t.Fatalf("%v: %d samples from %d bytes", fm, n, len(src))
			}
			want := make([]float64, n)
			Decode(want, src, fm)
			for i := range n {
				if !sameFloat(got[i], float32(want[i])) {
					t.Fatalf("%v: float32 sample %d = %v, float64 %v", fm, i, got[i], want[i])
				}
				if !fm.Float() && (want[i] < -1 || want[i] >= 1) {
					t.Fatalf("%v: sample %d = %v, outside [-1, 1)", fm, i, want[i])
				}
			}
		}
	})
}

// FuzzEncodeDecode encodes arbitrary float32 samples, read from the fuzzer's
// bytes, and checks that decoding gives back the nearest code: within half a
// step for an in-range sample, a rail for one past it, zero for NaN.
func FuzzEncodeDecode(f *testing.F) {
	f.Add(binary.LittleEndian.AppendUint32(nil, math.Float32bits(0.5)))
	f.Add(binary.LittleEndian.AppendUint32(nil, math.Float32bits(float32(math.NaN()))))
	f.Fuzz(func(t *testing.T, raw []byte) {
		src := make([]float32, len(raw)/4)
		Decode(src, raw, F32LE)
		for _, fm := range Formats() {
			out := make([]byte, len(src)*fm.Width())
			if n := Encode(out, src, fm); n != len(src) {
				t.Fatalf("%v: encoded %d of %d", fm, n, len(src))
			}
			back := make([]float64, len(src))
			Decode(back, out, fm)
			for i, v := range src {
				checkNearest(t, fm, float64(v), back[i])
			}
		}
	})
}

func checkNearest(t *testing.T, f Format, v, got float64) {
	t.Helper()
	if f.Float() {
		if !sameFloat(got, float64(float32(v))) && !sameFloat(got, v) {
			t.Fatalf("%v: %v decodes back as %v", f, v, got)
		}
		return
	}
	step := math.Ldexp(1, 1-f.Bits())
	top := 1 - step
	switch {
	case math.IsNaN(v):
		if got != 0 {
			t.Fatalf("%v: NaN decodes back as %v", f, got)
		}
	case v >= top:
		if got != top {
			t.Fatalf("%v: %v decodes back as %v, want the top code", f, v, got)
		}
	case v <= -1:
		if got != -1 {
			t.Fatalf("%v: %v decodes back as %v, want -1", f, v, got)
		}
	case math.Abs(got-v) > step/2:
		t.Fatalf("%v: %v decodes back as %v, more than half a step away", f, v, got)
	}
}
