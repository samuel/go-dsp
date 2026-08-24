package dspviz

import (
	"math"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

func TestImpulseAndStep(t *testing.T) {
	taps := []float64{0.5, -0.25, 0.125}
	p, err := NewFIRProcessor(taps, 8000)
	if err != nil {
		t.Fatal(err)
	}
	h := Impulse(p, 8)
	for i, want := range taps {
		if math.Abs(h[i]-want) > 1e-15 {
			t.Errorf("impulse[%d] = %v, want %v", i, h[i], want)
		}
	}
	for i := len(taps); i < len(h); i++ {
		if h[i] != 0 {
			t.Errorf("impulse[%d] = %v, want 0 past the taps", i, h[i])
		}
	}

	// The step response of an FIR filter is the running sum of its taps.
	s := Step(p, 8)
	var sum float64
	for i := range s {
		if i < len(taps) {
			sum += taps[i]
		}
		if math.Abs(s[i]-sum) > 1e-15 {
			t.Errorf("step[%d] = %v, want %v", i, s[i], sum)
		}
	}

	// Impulse must reset first, or the step above would still be ringing.
	if again := Impulse(p, 8); again[0] != taps[0] {
		t.Errorf("Impulse did not reset: [0] = %v, want %v", again[0], taps[0])
	}
}

// TestDCFilterStepDecay checks a black-box measurement against the closed form
// the filter's own tests use: a DC blocker's step response is a^n.
func TestDCFilterStepDecay(t *testing.T) {
	const a = 0.95
	f, err := dsp.NewDCFilter[float64](a)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewDCProcessor(f, 8000)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range Step(p, 60) {
		want := math.Pow(a, float64(i))
		if math.Abs(v-want) > 1e-12 {
			t.Fatalf("step[%d] = %v, want %v", i, v, want)
		}
	}
}

// TestPolyphaseImpulse checks the claim that a rate changer has one impulse
// response per phase rather than one impulse response. A boxcar decimator by
// four sums four consecutive samples, so wherever in the group the impulse
// lands it comes out in the same output sample -- and the responses differ only
// in which later phases are still zero, which is exactly the structure the
// pulse-train construction in the published test suites recovers the hard way.
func TestPolyphaseImpulse(t *testing.T) {
	const factor = 4
	f, err := dsp.NewBoxcarDecimator(factor)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewComplexDownsampleProcessor(f, 48000)
	if err != nil {
		t.Fatal(err)
	}
	if got := Phases(p); got != factor {
		t.Fatalf("Phases = %d, want %d", got, factor)
	}
	if in, out := p.Rates(); in != 48000 || out != 12000 {
		t.Fatalf("Rates = %v, %v; want 48000, 12000", in, out)
	}

	all := PolyphaseImpulse(p, 32)
	if len(all) != factor {
		t.Fatalf("got %d phases, want %d", len(all), factor)
	}
	for phase, h := range all {
		if len(h) != 32/factor {
			t.Errorf("phase %d: %d output samples, want %d", phase, len(h), 32/factor)
		}
		// The impulse falls in the first group whatever its phase within it, so
		// the whole of the energy lands in output sample 0.
		if math.Abs(h[0]-1) > 1e-6 {
			t.Errorf("phase %d: output[0] = %v, want 1", phase, h[0])
		}
		for i := 1; i < len(h); i++ {
			if h[i] != 0 {
				t.Errorf("phase %d: output[%d] = %v, want 0", phase, i, h[i])
			}
		}
	}

	// A processor that does not change the rate has exactly one phase.
	id, _ := Identity(48000)
	if got := Phases(id); got != 1 {
		t.Errorf("Phases of an identity = %d, want 1", got)
	}
}

// TestBoxcarDecimatorResponse measures the complex decimator against the closed
// form for a boxcar of M samples, |sin(pi*f*M/fs) / sin(pi*f/fs)|. It is the
// check that the black-box path stays right through a rate change -- there are
// no coefficients here for the exact path to fall back on.
func TestBoxcarDecimatorResponse(t *testing.T) {
	const factor, rate = 4, 48000.0
	f, err := dsp.NewBoxcarDecimator(factor)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewComplexDownsampleProcessor(f, rate)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(Coefficients); ok {
		t.Error("a rate changer must not claim to have a transfer function")
	}

	r, err := FreqResponse(p, LinearGrid(100, 5500, 24), ResponseOptions{Frame: 1 << 12})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != MethodTone {
		t.Errorf("method %v, want the tone path", r.Method)
	}
	for i, f := range r.Freq {
		w := math.Pi * f / rate
		want := math.Abs(math.Sin(w*factor) / math.Sin(w))
		got := math.Hypot(real(r.H[i]), imag(r.H[i]))
		if math.Abs(got-want) > 1e-4*want {
			t.Fatalf("%.1f Hz: |H| = %v, closed form says %v", f, got, want)
		}
	}
	// A decimator by a whole number, fed a frame divisible by it, still reads
	// back coherently, so nothing should have needed a window.
	for _, w := range r.Warnings {
		t.Errorf("unexpected warning: %s", w)
	}
}

// TestRationalDecimatorDC checks the property the filter's own comment claims
// and nothing has tested: dividing each output by the number of samples that
// actually went into it, rather than by the nominal ratio, is what keeps a
// non-integer ratio from beating against the input.
func TestRationalDecimatorDC(t *testing.T) {
	f, err := dsp.NewRationalBoxcarDecimator(48000, 44100)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewRationalDownsampleProcessor(f)
	if err != nil {
		t.Fatal(err)
	}
	if in, out := p.Rates(); in != 48000 || out != 44100 {
		t.Fatalf("Rates = %v, %v; want 48000, 44100", in, out)
	}
	for i, v := range Step(p, 20000) {
		if math.Abs(v-1) > 1e-6 {
			t.Fatalf("output[%d] = %v; DC in has to give DC out, with no beat "+
				"against the alternating group size", i, v)
		}
	}
}

func TestProcessorValidation(t *testing.T) {
	if _, err := NewFIRProcessor(nil, 8000); err == nil {
		t.Error("an FIR filter with no taps should be an error")
	}
	if _, err := NewFIRProcessor([]float64{1}, 0); err == nil {
		t.Error("a zero sample rate should be an error")
	}
	if _, err := NewIIRProcessor(nil, 8000); err == nil {
		t.Error("a nil filter should be an error")
	}
	if _, err := NewFuncProcessor(nil, nil, 1, 1); err == nil {
		t.Error("a nil function should be an error")
	}
	p, _ := Identity(8000)
	if _, err := FreqResponse(p, nil, ResponseOptions{}); err == nil {
		t.Error("no frequencies should be an error")
	}
	if _, err := FreqResponse(p, []float64{100}, ResponseOptions{Method: MethodExact}); err == nil {
		t.Error("MethodExact without coefficients should be an error")
	}
	if _, err := FreqResponse(nil, []float64{100}, ResponseOptions{}); err == nil {
		t.Error("a nil processor should be an error")
	}
}

func TestGrids(t *testing.T) {
	g := LinearGrid(0, 10, 11)
	for i, v := range g {
		if math.Abs(v-float64(i)) > 1e-12 {
			t.Errorf("LinearGrid[%d] = %v", i, v)
		}
	}
	l := LogGrid(10, 10000, 4)
	for i, want := range []float64{10, 100, 1000, 10000} {
		if math.Abs(l[i]-want) > 1e-9 {
			t.Errorf("LogGrid[%d] = %v, want %v", i, l[i], want)
		}
	}
	if LinearGrid(1, 2, 0) != nil || LogGrid(1, 2, 0) != nil {
		t.Error("a grid of no points should be nil")
	}
	if g := LinearGrid(5, 9, 1); len(g) != 1 || g[0] != 5 {
		t.Errorf("a grid of one point should be its lower bound, got %v", g)
	}
	defer func() {
		if recover() == nil {
			t.Error("a logarithmic grid through zero should panic")
		}
	}()
	LogGrid(0, 10, 4)
}

func TestCoherentFreqIsOdd(t *testing.T) {
	const rate, n = 48000.0, 1 << 12
	for _, f := range []float64{20, 997, 1000, 4321, 20000, 23999} {
		hz, k := CoherentFreq(f, rate, n)
		if k&1 == 0 {
			t.Errorf("%v Hz gave bin %d, which is even", f, k)
		}
		if k < 1 || k >= n/2 {
			t.Errorf("%v Hz gave bin %d, outside (0, n/2)", f, k)
		}
		if math.Abs(hz-float64(k)*rate/n) > 1e-12 {
			t.Errorf("%v Hz: returned %v, which is not bin %d", f, hz, k)
		}
		// The snapped frequency has to be within a bin of what was asked for,
		// or the response is being reported at the wrong place.
		if math.Abs(hz-f) > 2*rate/n {
			t.Errorf("%v Hz snapped all the way to %v", f, hz)
		}
	}
}
