package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// asymTaps is deliberately not symmetric, so a test that feeds an impulse
// catches a convolution running in the wrong direction. Symmetric taps, which
// is what a linear-phase design produces and therefore what one would reach for,
// would pass either way.
var asymTaps = []float64{0.5, -0.25, 0.125, 0.75, -0.0625}

func TestFIRImpulseResponseIsTheTaps(t *testing.T) {
	f, err := NewFIRFilter(asymTaps)
	if err != nil {
		t.Fatal(err)
	}
	src := make([]float64, 2*len(asymTaps))
	src[0] = 1
	dst := make([]float64, len(src))
	f.Filter(dst, src)
	for i := range src {
		var want float64
		if i < len(asymTaps) {
			want = asymTaps[i]
		}
		if dst[i] != want {
			t.Errorf("[%d] = %v, want %v", i, dst[i], want)
		}
	}
}

// TestFIRMatchesFreqz is the strongest check available: a complex exponential is
// an eigenvector of any LTI filter, so once the transient has passed the output
// is exactly H(w) times the input. H(w) is what Freqz evaluates from the same
// taps by a completely different route, so agreeing to 1e-12 says the filter
// implements the transfer function its own Coefficients advertises -- direction,
// tap order, history handling and all.
func TestFIRMatchesFreqz(t *testing.T) {
	const n = 512
	f, err := NewRealCoefFIRFilter[complex128](asymTaps)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []int{0, 1, 7, 64, 200, 255} {
		f.Reset()
		w := 2 * math.Pi * float64(k) / n
		want := Freqz(asymTaps, nil, []float64{w})[0]

		src := make([]complex128, n)
		for i := range src {
			// Reduce the phase in integers so the argument stays exact.
			m := int64(k) * int64(i) % int64(n)
			s, c := math.Sincos(2 * math.Pi * float64(m) / n)
			src[i] = complex(c, s)
		}
		dst := make([]complex128, n)
		f.Filter(dst, src)

		// Skip the transient: the first len(taps)-1 outputs see zeros.
		for i := len(asymTaps) - 1; i < n; i++ {
			got := dst[i] / src[i]
			if cmplx.Abs(got-want) > 1e-12 {
				t.Fatalf("k=%d [%d]: H = %v, Freqz says %v", k, i, got, want)
			}
		}
	}
}

// TestFIRStreamingMatchesWholeSignal requires exactly equal output whatever the
// block boundaries fall on, which is what the carried history is for. Not within
// a tolerance: the same multiplies happen in the same order either way, so any
// difference is a bug rather than rounding.
func TestFIRStreamingMatchesWholeSignal(t *testing.T) {
	src := make([]float64, 997)
	for i := range src {
		src[i] = math.Sin(float64(i)*0.37) + 0.25*math.Cos(float64(i)*1.9)
	}
	f, err := NewFIRFilter(asymTaps)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]float64, len(src))
	f.Filter(want, src)

	for _, block := range []int{1, 2, 3, len(asymTaps) - 1, len(asymTaps), len(asymTaps) + 1, 64, 1000} {
		f.Reset()
		got := make([]float64, len(src))
		for i := 0; i < len(src); i += block {
			end := min(i+block, len(src))
			f.Filter(got[i:end], src[i:end])
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("block=%d [%d] = %v, want %v", block, i, got[i], want[i])
			}
		}
	}
}

func TestFIRFilterOneMatchesBlock(t *testing.T) {
	src := make([]float64, 100)
	for i := range src {
		src[i] = math.Sin(float64(i) * 0.11)
	}
	f, _ := NewFIRFilter(asymTaps)
	want := make([]float64, len(src))
	f.Filter(want, src)

	f.Reset()
	for i, v := range src {
		if got := f.FilterOne(v); got != want[i] {
			t.Fatalf("[%d] = %v, want %v", i, got, want[i])
		}
	}
}

func TestFIRInPlace(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	f, _ := NewFIRFilter(asymTaps)
	want := make([]float64, len(src))
	f.Filter(want, src)

	f.Reset()
	got := append([]float64(nil), src...)
	f.Filter(got, got)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestFIRDecimatorMatchesEveryNth pins the decimator against the definition:
// filtering and then throwing samples away. Exactly equal, and across block
// boundaries that do not line up with the decimation grid, which is what the
// carried phase is for.
func TestFIRDecimatorMatchesEveryNth(t *testing.T) {
	src := make([]float64, 501)
	for i := range src {
		src[i] = math.Sin(float64(i)*0.23) - 0.4*math.Cos(float64(i)*0.77)
	}
	for _, factor := range []int{1, 2, 3, 4, 7, 16} {
		f, _ := NewFIRFilter(asymTaps)
		full := make([]float64, len(src))
		f.Filter(full, src)

		d, err := NewFIRDecimator(asymTaps, factor)
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range []int{len(src), 1, 3, factor - 1, factor, factor + 1, 1000} {
			if block < 1 {
				continue
			}
			d.Reset()
			var got []float64
			for i := 0; i < len(src); i += block {
				end := min(i+block, len(src))
				in := src[i:end]
				out := make([]float64, d.OutputLen(len(in)))
				if n := d.Filter(out, in); n != len(out) {
					t.Fatalf("factor=%d block=%d: wrote %d, OutputLen said %d", factor, block, n, len(out))
				}
				got = append(got, out...)
			}
			var want []float64
			for i := 0; i < len(full); i += factor {
				want = append(want, full[i])
			}
			if len(got) != len(want) {
				t.Fatalf("factor=%d block=%d: %d outputs, want %d", factor, block, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("factor=%d block=%d [%d] = %v, want %v", factor, block, i, got[i], want[i])
				}
			}
		}
	}
}

func TestFIRDecimatorInPlace(t *testing.T) {
	src := make([]float64, 64)
	for i := range src {
		src[i] = float64(i)
	}
	d, _ := NewFIRDecimator(asymTaps, 4)
	out := d.FilterInPlace(append([]float64(nil), src...))
	if len(out) != 16 {
		t.Fatalf("got %d outputs, want 16", len(out))
	}
	d.Reset()
	f, _ := NewFIRFilter(asymTaps)
	full := make([]float64, len(src))
	f.Filter(full, src)
	for i, v := range out {
		if v != full[i*4] {
			t.Errorf("[%d] = %v, want %v", i, v, full[i*4])
		}
	}
}

func TestFIRConstructorsValidate(t *testing.T) {
	if _, err := NewFIRFilter([]float64{}); err == nil {
		t.Error("no taps: want an error")
	}
	if _, err := NewRealCoefFIRFilter[complex128]([]float64(nil)); err == nil {
		t.Error("no taps, complex: want an error")
	}
	if _, err := NewFIRDecimator(asymTaps, 0); err == nil {
		t.Error("factor 0: want an error")
	}
	if _, err := NewFIRDecimator([]float64{}, 2); err == nil {
		t.Error("no taps, decimator: want an error")
	}
	if _, err := NewRealCoefFIRDecimator[complex128]([]float64(nil), 2); err == nil {
		t.Error("no taps, complex decimator: want an error")
	}
	if _, err := NewRealCoefFIRDecimator[complex128]([]float64{1}, 0); err == nil {
		t.Error("factor 0, complex decimator: want an error")
	}
	// The taps are copied, so mutating the caller's slice changes nothing.
	taps := []float64{1, 2, 3}
	f, _ := NewFIRFilter(taps)
	taps[0] = 99
	if b, _ := f.Coefficients(); b[0] != 1 {
		t.Errorf("taps were not copied: %v", b)
	}
	// Coefficients hands back a copy too.
	b, a := f.Coefficients()
	b[0] = 42
	if b2, _ := f.Coefficients(); b2[0] != 1 {
		t.Errorf("Coefficients aliased the filter: %v", b2)
	}
	if a != nil {
		t.Errorf("a = %v, want nil for an FIR filter", a)
	}
	// Len is what a caller sizing a measurement frame reads; a transform
	// shorter than the tap count cannot hold the impulse response.
	if got := f.Len(); got != len(taps) {
		t.Errorf("Len = %d, want %d", got, len(taps))
	}
}

// TestFIRLinearPhase checks the property the doc comment claims for a symmetric
// tap set: every frequency is delayed by the same (len(taps)-1)/2 samples.
func TestFIRLinearPhase(t *testing.T) {
	taps := []float64{0.1, 0.2, 0.4, 0.2, 0.1}
	b, _ := func() ([]float64, []float64) {
		f, _ := NewFIRFilter(taps)
		return f.Coefficients()
	}()
	w := make([]float64, 64)
	for i := range w {
		w[i] = math.Pi * float64(i) / float64(len(w))
	}
	want := float64(len(taps)-1) / 2
	for i, d := range GroupDelay(b, nil, w) {
		if math.Abs(d-want) > 1e-9 {
			t.Errorf("w=%v: group delay %v, want %v", w[i], d, want)
		}
	}
}

func BenchmarkFIRFilter(b *testing.B) {
	for _, taps := range []int{15, 63, 255} {
		t := make([]float64, taps)
		for i := range t {
			t[i] = 1 / float64(taps)
		}
		f, _ := NewFIRFilter(t)
		src := make([]float64, benchSize)
		dst := make([]float64, benchSize)
		b.Run(map[int]string{15: "15taps", 63: "63taps", 255: "255taps"}[taps], func(b *testing.B) {
			b.SetBytes(int64(benchSize * 8))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				f.Filter(dst, src)
			}
		})
	}
}

// TestRealCoefFIRDecimator checks the complex-over-real-taps decimator against
// the same taps run as an ordinary FIRDecimator over the real part, which is
// the only claim the constructor makes: it widens the taps and changes nothing
// else. Factor is checked here because nothing else reads it -- dspviz derives
// an output rate from it, and a wrong one would only show as a mislabelled
// axis.
func TestRealCoefFIRDecimator(t *testing.T) {
	const factor = 3
	// Dyadic taps over integer samples, so every product and every partial sum
	// is exact and the two dot products have to agree bit for bit. They would
	// not otherwise: a real accumulation contracts into one FMA on arm64 while
	// the complex one rounds the product before adding it.
	taps := []float64{0.25, 0.5, 0.25, -0.125}

	cd, err := NewRealCoefFIRDecimator[complex128](taps, factor)
	if err != nil {
		t.Fatal(err)
	}
	if cd.Factor() != factor {
		t.Errorf("Factor = %d, want %d", cd.Factor(), factor)
	}
	rd, err := NewFIRDecimator(taps, factor)
	if err != nil {
		t.Fatal(err)
	}

	src := make([]float64, 64)
	csrc := make([]complex128, len(src))
	for i := range src {
		src[i] = float64(i%23 - 11)
		// A distinct imaginary part, so a decimator that dropped it or mixed
		// the two halves could not pass.
		csrc[i] = complex(src[i], -2*src[i])
	}

	if got, want := cd.OutputLen(len(csrc)), rd.OutputLen(len(src)); got != want {
		t.Fatalf("OutputLen = %d, want %d", got, want)
	}
	cdst := make([]complex128, cd.OutputLen(len(csrc)))
	rdst := make([]float64, rd.OutputLen(len(src)))
	if n, want := cd.Filter(cdst, csrc), rd.Filter(rdst, src); n != want {
		t.Fatalf("Filter wrote %d, want %d", n, want)
	}
	for i := range rdst {
		if real(cdst[i]) != rdst[i] || imag(cdst[i]) != -2*rdst[i] {
			t.Fatalf("[%d] = %v, want %v", i, cdst[i], complex(rdst[i], -2*rdst[i]))
		}
	}
}
