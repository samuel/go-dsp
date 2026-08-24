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

// response returns |H(w)| for the filter, evaluated on the unit circle at the
// normalized angular frequency w.
func response(f *BiQuadFilter[float64], w float64) float64 {
	b, a := f.Coefficients()
	return cmplx.Abs(Freqz(b, a, []float64{w})[0])
}

// TestBiQuadResponses checks each constructor against the gain the Audio EQ
// Cookbook promises at DC, at the corner frequency and at Nyquist. That is
// independent of how the coefficients are written, so it catches a transcribed
// sign or a swapped term, which comparing the coefficients against the same
// formulas would not.
func TestBiQuadResponses(t *testing.T) {
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
		filter *BiQuadFilter[float64]
		at     []float64 // frequencies in radians per sample
		want   []float64
	}{
		{"lowpass", must(NewLowPassBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, math.Pi}, []float64{1, 0}},
		{"highpass", must(NewHighPassBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, math.Pi}, []float64{0, 1}},
		// Constant skirt gain makes the peak gain equal to q.
		{"bandpass-skirt", must(NewBandPassConstantSkirtGainBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{0, q, 0}},
		{"bandpass-peak", must(NewBandPassConstantPeakGainBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{0, 1, 0}},
		{"notch", must(NewNotchBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, w0, math.Pi}, []float64{1, 0, 1}},
		{"allpass", must(NewAllPassBiQuad[float64](sampleRate, freq, q)),
			[]float64{0, w0 / 2, w0, 2 * w0, math.Pi}, []float64{1, 1, 1, 1, 1}},
		{"peakingeq", must(NewPeakingEQBiQuad[float64](sampleRate, freq, q, dbGain)),
			[]float64{0, w0, math.Pi}, []float64{1, gain, 1}},
		{"lowshelf", must(NewLowShelfBiQuad[float64](sampleRate, freq, slope, dbGain)),
			[]float64{0, math.Pi}, []float64{gain, 1}},
		{"highshelf", must(NewHighShelfBiQuad[float64](sampleRate, freq, slope, dbGain)),
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

// TestBiQuadFilterZeroValuePanics pins the zero value's behavior. Every
// coefficient is divided by A0, so A0 == 0 used to turn the whole stream into
// NaN without a word.
func TestBiQuadFilterZeroValuePanics(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"float64", func() {
			var f BiQuadFilter[float64]
			f.Filter(make([]float64, 4), make([]float64, 4))
		}},
		{"float32", func() {
			var f BiQuadFilter[float32]
			f.Filter(make([]float32, 4), make([]float32, 4))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic")
				}
			}()
			tc.run()
		})
	}
}

// TestBiQuadFilterWidthsAgree checks that the two instantiations agree, since
// both keep float64 delay lines and only the samples differ in width.
func TestBiQuadFilterWidthsAgree(t *testing.T) {
	src := rampAndTone(64)
	in32 := make([]float32, len(src))
	for i, v := range src {
		in32[i] = float32(v)
	}

	out := make([]float64, len(src))
	must(NewLowPassBiQuad[float64](48000, 1000, 0.7071)).Filter(out, src)
	out32 := make([]float32, len(src))
	must(NewLowPassBiQuad[float32](48000, 1000, 0.7071)).Filter(out32, in32)

	for i := range out {
		if math.Abs(float64(out32[i])-out[i]) > 1e-5 {
			t.Fatalf("sample %d: float32 %v, float64 %v", i, out32[i], out[i])
		}
	}
}

// TestBiQuadFilterDCBlocked checks a high-pass actually removes a constant, as
// a plain end-to-end check that Filter runs the recursion the right way round.
func TestBiQuadFilterDCBlocked(t *testing.T) {
	src := make([]float64, 4096)
	for i := range src {
		src[i] = 1
	}
	dst := make([]float64, len(src))
	must(NewHighPassBiQuad[float64](48000, 1000, 0.7071)).Filter(dst, src)
	if tail := dst[len(dst)-1]; math.Abs(tail) > 1e-6 {
		t.Errorf("high-pass settled at %v on a constant src, want 0", tail)
	}
}

// TestNewBiQuad covers the general constructor, which is the escape hatch for a
// response none of the named ones cover, and its one validation.
func TestNewBiQuad(t *testing.T) {
	// A two-sample moving average: y[n] = (x[n] + x[n-1])/2.
	f, err := NewBiQuad[float64](0.5, 0.5, 0, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float64, 4)
	f.Filter(out, []float64{1, 1, 1, 1})
	for i, want := range []float64{0.5, 1, 1, 1} {
		if out[i] != want {
			t.Errorf("[%d] = %v, want %v", i, out[i], want)
		}
	}

	_, err = NewBiQuad[float64](1, 0, 0, 0, 0, 0)
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
		if _, err := NewBiQuad[float64](tc.b0, tc.b1, tc.b2, tc.a0, tc.a1, tc.a2); err == nil {
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
	new  func(sampleRate, freq, shape float64) (*BiQuadFilter[float64], error)
}

func biquadCtors() []biquadCtor {
	return []biquadCtor{
		{"lowpass", NewLowPassBiQuad[float64]},
		{"highpass", NewHighPassBiQuad[float64]},
		{"bandpass-skirt", NewBandPassConstantSkirtGainBiQuad[float64]},
		{"bandpass-peak", NewBandPassConstantPeakGainBiQuad[float64]},
		{"notch", NewNotchBiQuad[float64]},
		{"allpass", NewAllPassBiQuad[float64]},
		{"peakingeq", func(rate, freq, q float64) (*BiQuadFilter[float64], error) {
			return NewPeakingEQBiQuad[float64](rate, freq, q, 6)
		}},
		{"lowshelf", func(rate, freq, slope float64) (*BiQuadFilter[float64], error) {
			return NewLowShelfBiQuad[float64](rate, freq, slope, 6)
		}},
		{"highshelf", func(rate, freq, slope float64) (*BiQuadFilter[float64], error) {
			return NewHighShelfBiQuad[float64](rate, freq, slope, 6)
		}},
	}
}

// TestBiQuadConstructorsReject sweeps every response constructor over the
// arguments checkBiQuadArgs exists to catch. Each one used to return a filter
// full of NaNs or a response other than the one asked for, and the sweep is
// over all nine because the checks live in a shared helper each of them has to
// remember to call.
//
// The valid case runs first, so a constructor that rejects everything cannot
// pass this test by accident.
func TestBiQuadConstructorsReject(t *testing.T) {
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
// warns about, which checkBiQuadArgs cannot see: a slope past 1 drives the
// square root in alpha negative, so the coefficients come out NaN and NewBiQuad
// is what catches it.
func TestShelfSlopeAboveOneRejected(t *testing.T) {
	if _, err := NewLowShelfBiQuad[float64](48000, 1000, 100, 6); err == nil {
		t.Error("NewLowShelfBiQuad accepted a shelf slope of 100")
	}
	if _, err := NewHighShelfBiQuad[float64](48000, 1000, 100, 6); err == nil {
		t.Error("NewHighShelfBiQuad accepted a shelf slope of 100")
	}
}

// TestBiQuadFilterReset checks that a reset filter reproduces its first output,
// which is the only way to reuse one on a new stream.
func TestBiQuadFilterReset(t *testing.T) {
	src := rampAndTone(32)
	f := must(NewLowPassBiQuad[float64](48000, 1000, 0.7071))
	first := make([]float64, len(src))
	f.Filter(first, src)
	again := make([]float64, len(src))
	f.Reset()
	f.Filter(again, src)
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("sample %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}
}
