package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// TestBiquadCutoff checks Freqz against the closed forms the cookbook
// constructors are supposed to hit, rather than against another evaluation of
// the same coefficients. A Butterworth low-pass is down exactly 3.0103 dB at
// its cutoff, an all-pass is unity everywhere, and a notch is a true null.
func TestBiquadCutoff(t *testing.T) {
	const rate, cutoff = 48000.0, 1000.0
	f := cutoff / rate

	lp := must(NewBiquadLowPass[float64](rate, cutoff, math.Sqrt2/2))
	b, a := realBA(lp.RealCoefficients())
	h := Freqz(b, a, []float64{f})
	db := 20 * math.Log10(cmplx.Abs(h[0]))
	if math.Abs(db-(-3.0102999566398)) > 1e-9 {
		t.Errorf("low-pass at cutoff: %.12f dB, want -3.0103", db)
	}

	hp := must(NewBiquadHighPass[float64](rate, cutoff, math.Sqrt2/2))
	b, a = realBA(hp.RealCoefficients())
	h = Freqz(b, a, []float64{f})
	if db = 20 * math.Log10(cmplx.Abs(h[0])); math.Abs(db-(-3.0102999566398)) > 1e-9 {
		t.Errorf("high-pass at cutoff: %.12f dB, want -3.0103", db)
	}

	ap := must(NewBiquadAllPass[float64](rate, cutoff, 0.5))
	b, a = realBA(ap.RealCoefficients())
	grid := LinearGridFor(t, 0, 0.5, 257)
	for i, hv := range Freqz(b, a, grid) {
		if math.Abs(cmplx.Abs(hv)-1) > 1e-14 {
			t.Fatalf("all-pass at f=%v: |H| = %v, want 1 (point %d)", grid[i], cmplx.Abs(hv), i)
		}
	}

	nf := must(NewBiquadNotch[float64](rate, cutoff, 2))
	b, a = realBA(nf.RealCoefficients())
	h = Freqz(b, a, []float64{f})
	// A true null, to the depth the cancellation in the numerator allows: this
	// is the one limit Freqz's doc comment admits to, and -250 dB is far below
	// anything a filter under test would put here.
	if db = 20 * math.Log10(cmplx.Abs(h[0])); db > -250 {
		t.Errorf("notch at center: %.1f dB, want a true null", db)
	}
}

// LinearGridFor is a test helper; the exported grid builders live in the
// analysis package rather than here.
func LinearGridFor(t *testing.T, low, high float64, n int) []float64 {
	t.Helper()
	w := make([]float64, n)
	for i := range w {
		w[i] = low + (high-low)*float64(i)/float64(n-1)
	}
	return w
}

// TestFreqzFIR checks the no-denominator path against the definition, and that
// a nil a is the same as {1}.
func TestFreqzFIR(t *testing.T) {
	b := []float64{0.1, -0.25, 0.4, -0.25, 0.1}
	f := LinearGridFor(t, 0, 0.5, 64)
	got := Freqz(b, nil, f)
	unit := Freqz(b, []float64{1}, f)
	for i, fi := range f {
		var want complex128
		for n, c := range b {
			want += complex(c, 0) * cmplx.Exp(complex(0, -2*math.Pi*fi*float64(n)))
		}
		if cmplx.Abs(got[i]-want) > 1e-14 {
			t.Fatalf("f=%v: got %v, want %v", fi, got[i], want)
		}
		if got[i] != unit[i] {
			t.Fatalf("f=%v: nil a gave %v, a={1} gave %v", fi, got[i], unit[i])
		}
	}
}

// TestFreqzAgreesWithImpulseFFT pins the identity between the exact formula
// and the measured transform: for an FIR filter transformed at a length at
// least its tap count, an FFT of the impulse response is literally the same
// sum Freqz evaluates, so they have to agree to the arithmetic.
func TestFreqzAgreesWithImpulseFFT(t *testing.T) {
	taps := []float64{0.02, -0.05, 0.11, 0.37, 0.37, 0.11, -0.05, 0.02}
	const n = 256

	f, _ := NewFFT[complex128](n)
	spec := make([]complex128, n)
	f.ForwardReal(spec, taps)

	freq := make([]float64, n/2+1)
	for k := range freq {
		freq[k] = float64(k) / float64(n)
	}
	h := Freqz(taps, nil, freq)
	for k := range freq {
		if cmplx.Abs(h[k]-spec[k]) > 1e-13 {
			t.Fatalf("bin %d: Freqz %v, FFT %v", k, h[k], spec[k])
		}
	}
}

// TestGroupDelayLinearPhase is the sharpest single assertion available here: a
// symmetric FIR of n taps delays every frequency by exactly (n-1)/2 samples, so
// one comparison exercises the closed-form group delay, the transfer function
// and the symmetry of the coefficients at once.
func TestGroupDelayLinearPhase(t *testing.T) {
	for _, taps := range [][]float64{
		{0.02, -0.05, 0.11, 0.37, 0.37, 0.11, -0.05, 0.02}, // even length
		{0.05, 0.15, 0.6, 0.15, 0.05},                      // odd length
	} {
		want := float64(len(taps)-1) / 2
		f := LinearGridFor(t, 0.008, 0.19, 97) // away from the stopband nulls
		for i, d := range GroupDelay(taps, nil, f) {
			if math.Abs(d-want) > 1e-9 {
				t.Fatalf("%d taps at f=%v: delay %v, want %v", len(taps), f[i], d, want)
			}
		}
	}
}

// TestGroupDelayMatchesPhaseSlope checks the analytic delay against the
// finite difference of the unwrapped phase, which is the only form available
// when a filter is measured rather than described by coefficients.
func TestGroupDelayMatchesPhaseSlope(t *testing.T) {
	lp := must(NewBiquadLowPass[float64](48000, 3000, 0.7071))
	b, a := realBA(lp.RealCoefficients())

	const n = 20001
	f := LinearGridFor(t, 0.0016, 0.4, n)
	h := Freqz(b, a, f)
	phase := make([]float64, n)
	for i, v := range h {
		phase[i] = math.Atan2(imag(v), real(v))
	}
	Unwrap(phase)

	// The phase is in radians, so the slope is taken against 2*pi*f.
	want := GroupDelay(b, a, f)
	for i := 1; i < n-1; i++ {
		got := -(phase[i+1] - phase[i-1]) / (2 * math.Pi * (f[i+1] - f[i-1]))
		if math.Abs(got-want[i]) > 1e-6 {
			t.Fatalf("f=%v: slope %v, closed form %v", f[i], got, want[i])
		}
	}
}

func TestUnwrap(t *testing.T) {
	// A ramp steeper than 2*pi per step, wrapped, has to come back out.
	const n = 200
	raw := make([]float64, n)
	want := make([]float64, n)
	for i := range raw {
		p := -0.9 * float64(i)
		want[i] = p
		raw[i] = math.Atan2(math.Sin(p), math.Cos(p))
	}
	Unwrap(raw)
	// Unwrapping fixes the shape, not the branch the first point landed on.
	off := want[0] - raw[0]
	for i := range raw {
		if math.Abs(raw[i]+off-want[i]) > 1e-9 {
			t.Fatalf("[%d]: got %v, want %v", i, raw[i]+off, want[i])
		}
	}

	var short []float64
	Unwrap(short) // must not panic
	Unwrap([]float32{1})
}

// TestDCFilterCoefficients checks that the blocker's transfer function really
// describes the filter, by running a tone through it and comparing.
func TestDCFilterCoefficients(t *testing.T) {
	f, err := NewDCFilter[float64](0.95)
	if err != nil {
		t.Fatal(err)
	}
	b, a := realBA(f.RealCoefficients())

	// A tone completing a whole number of cycles in the window the response is
	// read from, so the correlation below sees no leakage and the comparison is
	// limited by the transient rather than by the measurement.
	const frame, k = 2048, 127
	h := Freqz(b, a, []float64{float64(k) / frame})[0]

	const settle = 8 * frame
	in := make([]float64, settle+frame)
	out := make([]float64, len(in))
	for i := range in {
		in[i] = math.Cos(2 * math.Pi * float64(k*i%frame) / frame)
	}
	f.Process(out, in)

	var re, im float64
	for i, v := range out[settle:] {
		s, c := math.Sincos(2 * math.Pi * float64(k*i%frame) / frame)
		re += v * c
		im -= v * s
	}
	got := 2 * complex(re, im) / complex(frame, 0)
	if cmplx.Abs(got-h) > 1e-12 {
		t.Errorf("measured H = %v, Freqz says %v", got, h)
	}
}
