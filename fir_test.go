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
	f.Process(dst, src)
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
		want := Freqz(asymTaps, nil, []float64{float64(k) / n})[0]

		src := make([]complex128, n)
		for i := range src {
			// Reduce the phase in integers so the argument stays exact.
			m := int64(k) * int64(i) % int64(n)
			s, c := math.Sincos(2 * math.Pi * float64(m) / n)
			src[i] = complex(c, s)
		}
		dst := make([]complex128, n)
		f.Process(dst, src)

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
	f.Process(want, src)

	for _, block := range []int{1, 2, 3, len(asymTaps) - 1, len(asymTaps), len(asymTaps) + 1, 64, 1000} {
		f.Reset()
		got := make([]float64, len(src))
		for i := 0; i < len(src); i += block {
			end := min(i+block, len(src))
			f.Process(got[i:end], src[i:end])
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("block=%d [%d] = %v, want %v", block, i, got[i], want[i])
			}
		}
	}
}

func TestFIRProcessOneMatchesBlock(t *testing.T) {
	src := make([]float64, 100)
	for i := range src {
		src[i] = math.Sin(float64(i) * 0.11)
	}
	f, _ := NewFIRFilter(asymTaps)
	want := make([]float64, len(src))
	f.Process(want, src)

	f.Reset()
	for i, v := range src {
		if got := f.ProcessOne(v); got != want[i] {
			t.Fatalf("[%d] = %v, want %v", i, got, want[i])
		}
	}
}

func TestFIRInPlace(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	f, _ := NewFIRFilter(asymTaps)
	want := make([]float64, len(src))
	f.Process(want, src)

	f.Reset()
	got := append([]float64(nil), src...)
	f.Process(got, got)
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
		f.Process(full, src)

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
				if n := d.Process(out, in); n != len(out) {
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
	out := inPlace(d, append([]float64(nil), src...))
	if len(out) != 16 {
		t.Fatalf("got %d outputs, want 16", len(out))
	}
	d.Reset()
	f, _ := NewFIRFilter(asymTaps)
	full := make([]float64, len(src))
	f.Process(full, src)
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
	if b := f.Taps(); b[0] != 1 {
		t.Errorf("taps were not copied: %v", b)
	}
	// Taps and RealCoefficients hand back copies too.
	f.Taps()[0] = 42
	b, a, ok := f.RealCoefficients()
	if !ok {
		t.Fatal("RealCoefficients: ok false for real taps")
	}
	b[0] = 42
	if b2 := f.Taps(); b2[0] != 1 {
		t.Errorf("Taps or RealCoefficients aliased the filter: %v", b2)
	}
	if a != nil {
		t.Errorf("a = %v, want nil for an FIR filter", a)
	}
	// Complex taps report by value.
	if b, _, ok := must(NewFIRFilter([]complex64{1, -2})).RealCoefficients(); !ok || b[1] != -2 {
		t.Errorf("complex type, real taps: RealCoefficients = (%v, %v), want ([1 -2], true)", b, ok)
	}
	if _, _, ok := must(NewFIRFilter([]complex64{1, 1i})).RealCoefficients(); ok {
		t.Error("complex taps: RealCoefficients ok true")
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
	b := must(NewFIRFilter(taps)).Taps()
	f := make([]float64, 64)
	for i := range f {
		f[i] = 0.5 * float64(i) / float64(len(f))
	}
	want := float64(len(taps)-1) / 2
	for i, d := range GroupDelay(b, nil, f) {
		if math.Abs(d-want) > 1e-9 {
			t.Errorf("f=%v: group delay %v, want %v", f[i], d, want)
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
				f.Process(dst, src)
			}
		})
	}
}

// TestRealCoefFIRDecimator checks the complex-over-real-taps decimator against
// the same taps run as an ordinary FIRDecimator over the real part, which is
// the only claim the constructor makes: it widens the taps and changes nothing
// else. Ratio is checked here because nothing else reads it -- dspviz derives
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
	if in, out := cd.Ratio(); in != factor || out != 1 {
		t.Errorf("Ratio = (%d, %d), want (%d, 1)", in, out, factor)
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
	if n, want := cd.Process(cdst, csrc), rd.Process(rdst, src); n != want {
		t.Fatalf("Process wrote %d, want %d", n, want)
	}
	for i := range rdst {
		if real(cdst[i]) != rdst[i] || imag(cdst[i]) != -2*rdst[i] {
			t.Fatalf("[%d] = %v, want %v", i, cdst[i], complex(rdst[i], -2*rdst[i]))
		}
	}
}

// mkSample returns re+im*i as T, dropping im for a real T.
func mkSample[T Sample](re, im float64) T {
	var v T
	switch p := any(&v).(type) {
	case *float32:
		*p = float32(re)
	case *float64:
		*p = re
	case *complex64:
		*p = complex64(complex(re, im))
	case *complex128:
		*p = complex(re, im)
	}
	return v
}

// sampleDiff returns |a-b| for any sample width.
func sampleDiff[T Sample](a, b T) float64 {
	switch d := any(a - b).(type) {
	case float32:
		return math.Abs(float64(d))
	case float64:
		return math.Abs(d)
	case complex64:
		return cmplx.Abs(complex128(d))
	case complex128:
		return cmplx.Abs(d)
	}
	panic("unreachable")
}

// testFIRInterpolatorMatchesStuffedFIR pins the interpolator against the
// definition: zero-stuffing and then filtering with every tap. Exactly equal,
// since the products that skip the stuffed zeros are only ever zero terms,
// and across block boundaries, which is what the carried history is for.
func testFIRInterpolatorMatchesStuffedFIR[T Sample](t *testing.T) {
	src := make([]T, 203)
	for i := range src {
		src[i] = mkSample[T](math.Sin(float64(i)*0.37), math.Cos(float64(i)*1.9))
	}
	for _, factor := range []int{1, 2, 3, 5} {
		for _, ntaps := range []int{1, 2, len(asymTaps), 12, 31} {
			taps := make([]T, ntaps)
			for k := range taps {
				taps[k] = mkSample[T](asymTaps[k%len(asymTaps)]*float64(k+1), 0)
			}
			ref := must(NewFIRFilter(taps))
			stuffed := make([]T, len(src)*factor)
			for i, v := range src {
				stuffed[i*factor] = v
			}
			want := make([]T, len(stuffed))
			ref.Process(want, stuffed)

			it := must(NewFIRInterpolator(taps, factor))
			if in, out := it.Ratio(); in != 1 || out != factor || it.OutputLen(7) != 7*factor {
				t.Fatalf("Ratio (%d, %d), OutputLen(7) %d", in, out, it.OutputLen(7))
			}
			for _, block := range []int{1, 2, 7, 64, len(src)} {
				it.Reset()
				got := make([]T, len(want))
				for i := 0; i < len(src); i += block {
					end := min(i+block, len(src))
					if n := it.Process(got[i*factor:], src[i:end]); n != (end-i)*factor {
						t.Fatalf("wrote %d, want %d", n, (end-i)*factor)
					}
				}
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("factor=%d taps=%d block=%d [%d] = %v, want %v",
							factor, ntaps, block, i, got[i], want[i])
					}
				}
			}
		}
	}
}

func TestFIRInterpolatorMatchesStuffedFIR(t *testing.T) {
	t.Run("float32", testFIRInterpolatorMatchesStuffedFIR[float32])
	t.Run("float64", testFIRInterpolatorMatchesStuffedFIR[float64])
	t.Run("complex64", testFIRInterpolatorMatchesStuffedFIR[complex64])
	t.Run("complex128", testFIRInterpolatorMatchesStuffedFIR[complex128])
}

// TestFIRInterpolatorSharedBuffer writes the output over the input it came
// from, which the doc comment allows because src is staged first.
func TestFIRInterpolatorSharedBuffer(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	it := must(NewFIRInterpolator(asymTaps, 3))
	want := make([]float64, 3*len(src))
	it.Process(want, src)

	it.Reset()
	buf := make([]float64, 3*len(src))
	copy(buf, src)
	it.Process(buf, buf[:len(src)])
	for i := range want {
		if buf[i] != want[i] {
			t.Fatalf("[%d] = %v, want %v", i, buf[i], want[i])
		}
	}
}

func TestFIRInterpolatorShortDstPanics(t *testing.T) {
	it := must(NewFIRInterpolator(asymTaps, 2))
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	it.Process(make([]float64, 7), make([]float64, 4))
}

func TestFIRInterpolatorConstructorsValidate(t *testing.T) {
	if _, err := NewFIRInterpolator(asymTaps, 0); err == nil {
		t.Error("factor 0: want an error")
	}
	if _, err := NewFIRInterpolator([]float64{}, 2); err == nil {
		t.Error("no taps: want an error")
	}
	if _, err := NewRealCoefFIRInterpolator[complex64]([]float32(nil), 2); err == nil {
		t.Error("no taps, complex: want an error")
	}
	if _, err := NewRealCoefFIRInterpolator[complex64]([]float32{1}, 0); err == nil {
		t.Error("factor 0, complex: want an error")
	}
	// The taps are copied.
	taps := []float64{1, 2, 3}
	it := must(NewFIRInterpolator(taps, 2))
	taps[0] = 99
	dst := make([]float64, 2)
	it.Process(dst, []float64{1})
	if dst[0] != 1 {
		t.Errorf("taps were not copied: first output %v", dst[0])
	}
}

// TestRealCoefFIRInterpolator checks that real taps over complex samples run
// the real filter over each part: the output is bit-identical to interpolating
// the real and imaginary parts separately.
func TestRealCoefFIRInterpolator(t *testing.T) {
	src := make([]complex128, 50)
	re := make([]float64, len(src))
	im := make([]float64, len(src))
	for i := range src {
		re[i], im[i] = math.Sin(float64(i)*0.3), math.Cos(float64(i)*0.7)
		src[i] = complex(re[i], im[i])
	}
	rc := must(NewRealCoefFIRInterpolator[complex128](asymTaps, 3))
	got := make([]complex128, 3*len(src))
	rc.Process(got, src)
	wantRe := make([]float64, 3*len(src))
	wantIm := make([]float64, 3*len(src))
	must(NewFIRInterpolator(asymTaps, 3)).Process(wantRe, re)
	must(NewFIRInterpolator(asymTaps, 3)).Process(wantIm, im)
	for i := range got {
		if real(got[i]) != wantRe[i] || imag(got[i]) != wantIm[i] {
			t.Fatalf("[%d] = %v, want (%v%+vi)", i, got[i], wantRe[i], wantIm[i])
		}
	}
}

func TestFIRInterpolatorOutputLenOverflowPanics(t *testing.T) {
	it := must(NewFIRInterpolator(asymTaps, 3))
	if got := it.OutputLen(math.MaxInt / 3); got != math.MaxInt/3*3 {
		t.Errorf("OutputLen(MaxInt/3) = %d", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	it.OutputLen(math.MaxInt/3 + 1)
}
