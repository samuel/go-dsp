package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func sineF64(n int, f float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.Sin(2 * math.Pi * f * float64(i))
	}
	return s
}

func TestResamplerFollowsTheSignal(t *testing.T) {
	const f = 0.01 // cycles per input sample, well inside every passband here
	in := sineF64(6000, f)
	for _, ratio := range []float64{1, 0.5, 2, 44100.0 / 48000, 48000.0 / 44100, 0.1} {
		r, err := NewResampler[float64](ratio)
		if err != nil {
			t.Fatal(err)
		}
		var out []float64
		buf := make([]float64, 4096)
		for off := 0; off < len(in); off += 333 {
			src := in[off:min(off+333, len(in))]
			n := r.Process(buf, src)
			out = append(out, buf[:n]...)
		}
		if want := float64(len(in)) * ratio; math.Abs(float64(len(out))-want) > float64(r.taps) {
			t.Errorf("ratio %v: %d outputs, want about %v", ratio, len(out), want)
		}
		// Skip the ends, where the filter reaches past the input.
		edge := int(float64(r.taps) * max(1, ratio))
		for k := edge; k < len(out)-edge; k++ {
			want := math.Sin(2 * math.Pi * f * float64(k) / ratio)
			if math.Abs(out[k]-want) > 2e-3 {
				t.Fatalf("ratio %v: out[%d] = %v, want %v", ratio, k, out[k], want)
			}
		}
	}
}

func TestResamplerBlockSizesAgree(t *testing.T) {
	in := sineF64(3000, 0.03)
	run := func(sizes []int) []float64 {
		r, err := NewResampler[float64](0.73)
		if err != nil {
			t.Fatal(err)
		}
		var out []float64
		for off, i := 0, 0; off < len(in); i++ {
			n := min(sizes[i%len(sizes)], len(in)-off)
			dst := make([]float64, r.OutputLen(n))
			if got := r.Process(dst, in[off:off+n]); got != len(dst) {
				t.Fatalf("Process wrote %d, OutputLen said %d", got, len(dst))
			}
			out = append(out, dst...)
			off += n
		}
		return out
	}
	a, b := run([]int{3000}), run([]int{1, 7, 100, 64, 3})
	if len(a) != len(b) {
		t.Fatalf("%d outputs in one block, %d in pieces", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("output %d differs: %v and %v", i, a[i], b[i])
		}
	}
}

func TestResamplerComplexAndSetRatio(t *testing.T) {
	r, err := NewResampler[complex64](1)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetRatio(1.001); err != nil || r.Ratio() != 1.001 {
		t.Fatalf("SetRatio: %v, ratio %v", err, r.Ratio())
	}
	in := make([]complex64, 2000)
	for i := range in {
		in[i] = complex64(cmplx.Rect(1, 2*math.Pi*0.02*float64(i)))
	}
	out := make([]complex64, r.OutputLen(len(in)))
	r.Process(out, in)
	if want := 2000 * 1.001; math.Abs(float64(len(out))-want) > 16 {
		t.Errorf("%d outputs, want about %v", len(out), want)
	}
	for k := 100; k < len(out)-100; k++ {
		if d := cmplx.Abs(complex128(out[k]) - cmplx.Rect(1, 2*math.Pi*0.02*float64(k)/1.001)); d > 2e-3 {
			t.Fatalf("out[%d] off by %v", k, d)
		}
	}
	for _, bad := range []float64{0, -1, math.Inf(1), math.NaN()} {
		if _, err := NewResampler[float32](bad); err == nil {
			t.Errorf("ratio %v accepted", bad)
		}
		if err := r.SetRatio(bad); err == nil {
			t.Errorf("SetRatio(%v) accepted", bad)
		}
	}
}
