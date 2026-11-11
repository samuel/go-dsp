package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// must unwraps a constructor that returns an error, for the call sites here
// that build a filter from arguments known to be valid. It panics rather than
// taking a *testing.T, because Go does not allow a multi-value call to sit
// alongside other arguments and most of these are inside table literals.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// realBA drops the ok of a RealCoefficients call on a filter known to be real.
func realBA(b, a []float64, ok bool) ([]float64, []float64) {
	if !ok {
		panic("coefficients are complex")
	}
	return b, a
}

// response returns |H(w)| for the filter, evaluated on the unit circle at the
// normalized angular frequency w.
func response(f *Biquad[float64], w float64) float64 {
	b, a := realBA(f.RealCoefficients())
	return cmplx.Abs(Freqz(b, a, []float64{w / (2 * math.Pi)})[0])
}

// TestBiquadResponses checks each constructor against the gain the Audio EQ
// Cookbook promises at DC, at the corner frequency and at Nyquist. That is
// independent of how the coefficients are written, so it catches a transcribed
// sign or a swapped term, which comparing the coefficients against the same
// formulas would not.
func TestBiquadResponses(t *testing.T) {
	const (
		sampleRate = 48000
		freq       = 1000
		q          = 0.7071
		slope      = 1
		dbGain     = 6.0
	)
	w0 := 2 * math.Pi * freq / sampleRate
	gain := math.Pow(10, dbGain/20) // the linear gain the shelves and the peak reach

	for _, tc := range []struct {
		name   string
		filter *Biquad[float64]
		at     []float64 // frequencies in radians per sample
		want   []float64
	}{
		{"lowpass", must(NewBiquadLowPass[float64](sampleRate, freq, q)),
			[]float64{0, math.Pi}, []float64{1, 0}},
		{"highpass", must(NewBiquadHighPass[float64](sampleRate, freq, q)),
			[]float64{0, math.Pi}, []float64{0, 1}},
		// Constant skirt gain makes the peak gain equal to q.
		{"bandpass-skirt", must(NewBiquadBandPassConstantSkirtGain[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{0, q, 0}},
		{"bandpass-peak", must(NewBiquadBandPassConstantPeakGain[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{0, 1, 0}},
		{"notch", must(NewBiquadNotch[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{1, 0, 1}},
		{"allpass", must(NewBiquadAllPass[float64](sampleRate, freq, q)),
			[]float64{0, w0 / 2, w0, 2 * w0, math.Pi}, []float64{1, 1, 1, 1, 1}},
		{"peakingeq", must(NewBiquadPeakingEQ[float64](sampleRate, freq, q, dbGain)),
			[]float64{0, w0, math.Pi}, []float64{1, gain, 1}},
		{"lowshelf", must(NewBiquadLowShelf[float64](sampleRate, freq, slope, dbGain)),
			[]float64{0, math.Pi}, []float64{gain, 1}},
		{"highshelf", must(NewBiquadHighShelf[float64](sampleRate, freq, slope, dbGain)),
			[]float64{0, math.Pi}, []float64{1, gain}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, w := range tc.at {
				got := response(tc.filter, w)
				if math.Abs(got-tc.want[i]) > 1e-9 {
					t.Errorf("|H(%v)| = %v, want %v", w, got, tc.want[i])
				}
			}
		})
	}
}

// TestBiquadZeroValueIsSilent pins the zero value's behavior: every
// coefficient is zero, so it outputs silence rather than NaN.
func TestBiquadZeroValueIsSilent(t *testing.T) {
	var f Biquad[float64]
	out := []float64{9, 9, 9, 9}
	f.Process(out, []float64{1, -2, 3, -4})
	for i, v := range out {
		if v != 0 {
			t.Errorf("[%d] = %v, want 0", i, v)
		}
	}
}

// TestBiquadRealCoefficientsNormalized checks that the coefficients come back
// divided by a0, with a[0] exactly 1, as IIRFilter's do.
func TestBiquadRealCoefficientsNormalized(t *testing.T) {
	f := must(NewBiquad[float64](1, 0.5, -0.25, 4, 2, -1))
	b, a, ok := f.RealCoefficients()
	if !ok {
		t.Fatal("ok false")
	}
	wantSlice(t, "b", b, []float64{0.25, 0.125, -0.0625})
	wantSlice(t, "a", a, []float64{1, 0.5, -0.25})
}

// TestBiquadFilterWidthsAgree checks that the two instantiations agree, since
// both keep float64 delay lines and only the samples differ in width.
func TestBiquadFilterWidthsAgree(t *testing.T) {
	src := rampAndTone(64)
	in32 := make([]float32, len(src))
	for i, v := range src {
		in32[i] = float32(v)
	}

	out := make([]float64, len(src))
	must(NewBiquadLowPass[float64](48000, 1000, 0.7071)).Process(out, src)
	out32 := make([]float32, len(src))
	must(NewBiquadLowPass[float32](48000, 1000, 0.7071)).Process(out32, in32)

	for i := range out {
		if math.Abs(float64(out32[i])-out[i]) > 1e-5 {
			t.Fatalf("sample %d: float32 %v, float64 %v", i, out32[i], out[i])
		}
	}
}

// TestBiquadFilterDCBlocked checks a high-pass actually removes a constant, as
// a plain end-to-end check that Process runs the recursion the right way round.
func TestBiquadFilterDCBlocked(t *testing.T) {
	src := make([]float64, 4096)
	for i := range src {
		src[i] = 1
	}
	dst := make([]float64, len(src))
	must(NewBiquadHighPass[float64](48000, 1000, 0.7071)).Process(dst, src)
	if tail := dst[len(dst)-1]; math.Abs(tail) > 1e-6 {
		t.Errorf("high-pass settled at %v on a constant src, want 0", tail)
	}
}

// TestNewBiquad covers the general constructor, which is the escape hatch for a
// response none of the named ones cover, and its one validation.
func TestNewBiquad(t *testing.T) {
	// A two-sample moving average: y[n] = (x[n] + x[n-1])/2.
	f, err := NewBiquad[float64](0.5, 0.5, 0, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float64, 4)
	f.Process(out, []float64{1, 1, 1, 1})
	for i, want := range []float64{0.5, 1, 1, 1} {
		if out[i] != want {
			t.Errorf("[%d] = %v, want %v", i, out[i], want)
		}
	}

	_, err = NewBiquad[float64](1, 0, 0, 0, 0, 0)
	if err == nil {
		t.Fatal("expected an error for a zero A0")
	}

	// A coefficient that is not finite once divided by A0. The recursion feeds
	// its output back, so one of these turns the whole stream into NaN from the
	// first sample rather than degrading gracefully.
	for _, tc := range []struct {
		name                   string
		b0, b1, b2, a0, a1, a2 float64
	}{
		{"NaN numerator", math.NaN(), 0, 0, 1, 0, 0},
		{"Inf numerator", math.Inf(1), 0, 0, 1, 0, 0},
		{"NaN denominator", 1, 0, 0, math.NaN(), 0, 0},
		{"overflows on divide", math.MaxFloat64, 0, 0, math.SmallestNonzeroFloat64, 0, 0},
	} {
		if _, err := NewBiquad[float64](tc.b0, tc.b1, tc.b2, tc.a0, tc.a1, tc.a2); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

// biquadCtor adapts every response constructor to one signature: a sample rate,
// a frequency and the shape argument, which is Q for the resonant responses and
// the shelf slope for the two shelves. The gain the two shelves and the peaking
// EQ take is fixed, since it is the one argument none of them validates.
type biquadCtor struct {
	name string
	new  func(sampleRate, freq, shape float64) (*Biquad[float64], error)
}

func biquadCtors() []biquadCtor {
	return []biquadCtor{
		{"lowpass", NewBiquadLowPass[float64]},
		{"highpass", NewBiquadHighPass[float64]},
		{"bandpass-skirt", NewBiquadBandPassConstantSkirtGain[float64]},
		{"bandpass-peak", NewBiquadBandPassConstantPeakGain[float64]},
		{"notch", NewBiquadNotch[float64]},
		{"allpass", NewBiquadAllPass[float64]},
		{"peakingeq", func(rate, freq, q float64) (*Biquad[float64], error) {
			return NewBiquadPeakingEQ[float64](rate, freq, q, 6)
		}},
		{"lowshelf", func(rate, freq, slope float64) (*Biquad[float64], error) {
			return NewBiquadLowShelf[float64](rate, freq, slope, 6)
		}},
		{"highshelf", func(rate, freq, slope float64) (*Biquad[float64], error) {
			return NewBiquadHighShelf[float64](rate, freq, slope, 6)
		}},
	}
}

// TestBiquadConstructorsReject sweeps every response constructor over the
// arguments checkBiquadArgs exists to catch. Each one used to return a filter
// full of NaNs or a response other than the one asked for, and the sweep is
// over all nine because the checks live in a shared helper each of them has to
// remember to call.
//
// The valid case runs first, so a constructor that rejects everything cannot
// pass this test by accident.
func TestBiquadConstructorsReject(t *testing.T) {
	const rate = 48000
	bad := []struct {
		name              string
		rate, freq, shape float64
	}{
		{"zero rate", 0, 1000, 0.7071},
		{"negative rate", -rate, 1000, 0.7071},
		{"NaN rate", math.NaN(), 1000, 0.7071},
		{"zero freq", rate, 0, 0.7071},
		{"negative freq", rate, -1000, 0.7071},
		{"freq at Nyquist", rate, rate / 2, 0.7071},
		{"freq above Nyquist", rate, rate, 0.7071},
		{"NaN freq", rate, math.NaN(), 0.7071},
		{"zero shape", rate, 1000, 0},
		{"negative shape", rate, 1000, -1},
		{"NaN shape", rate, 1000, math.NaN()},
	}
	for _, c := range biquadCtors() {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.new(rate, 1000, 0.7071); err != nil {
				t.Fatalf("valid arguments: %v", err)
			}
			for _, tc := range bad {
				if _, err := c.new(tc.rate, tc.freq, tc.shape); err == nil {
					t.Errorf("%s: expected an error", tc.name)
				}
			}
		})
	}
}

// TestShelfSlopeAboveOneRejected covers the failure the shelf doc comment
// warns about, which checkBiquadArgs cannot see: a slope past 1 drives the
// square root in alpha negative, so the coefficients come out NaN and NewBiquad
// is what catches it.
func TestShelfSlopeAboveOneRejected(t *testing.T) {
	if _, err := NewBiquadLowShelf[float64](48000, 1000, 100, 6); err == nil {
		t.Error("NewBiquadLowShelf accepted a shelf slope of 100")
	}
	if _, err := NewBiquadHighShelf[float64](48000, 1000, 100, 6); err == nil {
		t.Error("NewBiquadHighShelf accepted a shelf slope of 100")
	}
}

// TestBiquadFilterReset checks that a reset filter reproduces its first output,
// which is the only way to reuse one on a new stream.
func TestBiquadFilterReset(t *testing.T) {
	src := rampAndTone(32)
	f := must(NewBiquadLowPass[float64](48000, 1000, 0.7071))
	first := make([]float64, len(src))
	f.Process(first, src)
	again := make([]float64, len(src))
	f.Reset()
	f.Process(again, src)
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("sample %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}
}

// TestBiquadProcessOneMatchesProcess requires the two entry points to agree bit
// for bit, not merely closely: the delay lines feed back, so a one-ulp
// difference would compound. The two share step, which is what makes an exact
// comparison the right one.
func TestBiquadProcessOneMatchesProcess(t *testing.T) {
	src := rampAndTone(256)

	for _, tc := range []struct {
		name  string
		build func() (*Biquad[float64], error)
	}{
		{"lowpass", func() (*Biquad[float64], error) {
			return NewBiquadLowPass[float64](48000, 1000, 0.7071)
		}},
		{"peakingeq", func() (*Biquad[float64], error) {
			return NewBiquadPeakingEQ[float64](48000, 3000, 4, 12)
		}},
		// a0 far from 1.
		{"unnormalized", func() (*Biquad[float64], error) {
			return NewBiquad[float64](0.3, 0.7, 0.11, 3.7, -1.3, 0.29)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block, err := tc.build()
			if err != nil {
				t.Fatal(err)
			}
			single, err := tc.build()
			if err != nil {
				t.Fatal(err)
			}

			dst := make([]float64, len(src))
			block.Process(dst, src)
			for i, s := range src {
				if got := single.ProcessOne(s); got != dst[i] {
					t.Fatalf("sample %d: ProcessOne %v, Process %v (differ by %g)",
						i, got, dst[i], math.Abs(got-dst[i]))
				}
			}
		})
	}
}
