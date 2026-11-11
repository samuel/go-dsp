package analysis

import (
	"math"
	"slices"
	"testing"

	"samuelks.com/dsp"
)

// TestTHDNCountsEverythingTHDDoesNot is the distinction between the two
// figures, which is easy to state and easy to get wrong: THD sums the
// harmonics it was asked for and THDN sums every bin that is not the
// fundamental. A processor that adds broadband noise but no harmonic therefore
// moves one and not the other.
func TestTHDNCountsEverythingTHDDoesNot(t *testing.T) {
	const rate = 48000.0
	// A 12-bit quantizer: its error is spread over the whole band rather than
	// concentrated on a harmonic, and it is arithmetic rather than random.
	const steps = 1 << 11
	p := processorFunc(func(dst, src []float64) []float64 {
		for _, x := range src {
			dst = append(dst, math.Round(x*steps)/steps)
		}
		return dst
	}, nil)
	s, err := ToneSpectrum(p, rate, 1000, SpectrumOptions{Frame: 1 << 13})
	if err != nil {
		t.Fatal(err)
	}
	thd, thdn := s.THD(10), s.THDN()
	if !(thdn > thd+10) {
		t.Errorf("THD %.1f dB, THD+N %.1f dB: quantization noise should raise the second well above the first", thd, thdn)
	}
	// And THD+N cannot be quieter than the worst single spur it contains.
	if sfdr := s.SFDR(); thdn < sfdr {
		t.Errorf("THD+N %.1f dB is below the SFDR %.1f dB, which it sums over", thdn, sfdr)
	}

	// Fewer than two harmonics is not a distortion measurement.
	if got := s.THD(1); !math.IsInf(got, -1) {
		t.Errorf("THD(1) = %v, want -Inf", got)
	}
	// An empty spectrum reports no distortion rather than dividing by nothing.
	empty := &Spectrum{Rate: rate}
	if got := empty.THDN(); !math.IsInf(got, -1) {
		t.Errorf("THDN of an empty spectrum = %v, want -Inf", got)
	}
	if got := empty.THD(10); !math.IsInf(got, -1) {
		t.Errorf("THD of an empty spectrum = %v, want -Inf", got)
	}
	if got := empty.SFDR(); got != 0 {
		t.Errorf("SFDR of an empty spectrum = %v, want 0", got)
	}
}

// TestGroupDelaySecondsMatchesSamples pins the unit conversion, which is the
// whole of that method and is exactly the kind of thing that reads correctly
// while being off by the rate.
func TestGroupDelaySecondsMatchesSamples(t *testing.T) {
	const rate = 48000.0
	taps := make([]float64, 33)
	for i := range taps {
		taps[i] = 1.0 / float64(len(taps))
	}
	p := must(dsp.NewFIRFilter(taps))
	r, err := FreqResponse(p, rate, LinearGrid(100, 6000, 64), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	samples := r.GroupDelay()
	seconds := r.GroupDelaySeconds()
	if len(seconds) != len(samples) {
		t.Fatalf("%d seconds against %d samples", len(seconds), len(samples))
	}
	for i := range samples {
		if want := samples[i] / rate; seconds[i] != want {
			t.Fatalf("[%d] = %v s, want %v", i, seconds[i], want)
		}
	}
	// A moving average of 33 taps delays by 16 samples, which is a third of a
	// millisecond at this rate. Checking the absolute value as well as the
	// ratio catches the two going wrong together.
	if want := 16.0 / rate; math.Abs(seconds[0]-want) > 1e-9 {
		t.Errorf("group delay %v s, want %v", seconds[0], want)
	}
}

// TestGridWarningFiresOnACoarseGrid covers both sides of the test that decides
// whether an unwrapped phase can be trusted: a filter whose delay turns the
// phase by more than pi between neighbors has lost turns that no unwrapping can
// put back, and the warning is the only thing that says so.
//
// The processor is a long FIR because it reports coefficients, so the response
// is measured exactly and its group delay is the closed form. That is what the
// warning needs: a differenced delay cannot exceed pi per grid step by
// construction, so a response measured by tone or impulse never warns however
// coarse its grid -- see the note on GroupDelay.
func TestGridWarningFiresOnACoarseGrid(t *testing.T) {
	const rate = 48000.0
	// 401 taps delays by 200 samples, so the phase turns by pi over a grid step
	// of pi/200 radians per sample -- about 120 Hz at this rate.
	taps := make([]float64, 401)
	for i := range taps {
		taps[i] = 1.0 / float64(len(taps))
	}
	p := must(dsp.NewFIRFilter(taps))

	fine, err := FreqResponse(p, rate, LinearGrid(100, 6000, 512), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fine.Method != MethodExact {
		t.Fatalf("measured by %v, so the delay is differenced and this test proves nothing", fine.Method)
	}
	if w := fine.GridWarning(); w != "" {
		t.Errorf("a grid stepping 11.5 Hz against a 200-sample delay warned: %s", w)
	}

	coarse, err := FreqResponse(p, rate, LinearGrid(100, 20000, 8), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if coarse.GridWarning() == "" {
		t.Error("a grid stepping 2800 Hz against a 200-sample delay did not warn")
	}

	// A response with nothing in it, and one whose group delay is zero, both
	// have nothing to warn about rather than dividing by nothing.
	if w := (&Response{Rate: rate}).GridWarning(); w != "" {
		t.Errorf("an empty response warned: %s", w)
	}
	flat := &Response{Rate: rate, Freq: []float64{100, 200}, H: []complex128{1, 1}}
	if w := flat.GridWarning(); w != "" {
		t.Errorf("a zero-delay response warned: %s", w)
	}

	// The closed form is the point of measuring exactly, so check it against
	// the delay the tap count implies rather than only against the warning.
	// The tolerance is loose at the nulls, where the recurrence divides by a
	// polynomial that is very nearly zero.
	for i, d := range fine.GroupDelay() {
		if math.Abs(d-200) > 1e-6 {
			t.Fatalf("group delay at %.0f Hz is %v samples, want 200", fine.Freq[i], d)
		}
	}
}

// TestImpulseAtGuardsItsArguments covers the edges of the phase argument, which
// PolyphaseImpulse drives from a rate ratio and a caller can drive from
// anywhere.
func TestImpulseAtGuardsItsArguments(t *testing.T) {
	taps := []float64{0.25, 0.5, 0.25}
	p := must(dsp.NewFIRFilter(taps))

	if got := ImpulseAt(p, 0, 0); got != nil {
		t.Errorf("n=0 returned %v, want nil", got)
	}
	if got := ImpulseAt(p, -4, 0); got != nil {
		t.Errorf("n=-4 returned %v, want nil", got)
	}

	// Phase 0 is the taps themselves; a negative phase is clamped to it rather
	// than indexing backwards.
	want := []float64{0.25, 0.5, 0.25, 0, 0, 0, 0, 0}
	for _, phase := range []int{0, -1, -100} {
		got := ImpulseAt(p, len(want), phase)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("phase %d: [%d] = %v, want %v (%v)", phase, i, got[i], want[i], got)
			}
		}
	}

	// A phase inside the frame shifts the response by that much.
	got := ImpulseAt(p, 8, 3)
	shifted := []float64{0, 0, 0, 0.25, 0.5, 0.25, 0, 0}
	for i := range shifted {
		if got[i] != shifted[i] {
			t.Fatalf("phase 3: [%d] = %v, want %v (%v)", i, got[i], shifted[i], got)
		}
	}

	// A phase past the frame puts no impulse in at all, so the response is
	// silence rather than a wrapped copy.
	for i, v := range ImpulseAt(p, 8, 8) {
		if v != 0 {
			t.Fatalf("phase past the frame: [%d] = %v, want 0", i, v)
		}
	}

	// Each call resets the processor, so the same phase twice gives the same
	// answer with no history carried between them.
	a := ImpulseAt(p, 8, 2)
	b := ImpulseAt(p, 8, 2)
	if !slices.Equal(a, b) {
		t.Errorf("two identical calls gave %v and %v", a, b)
	}
}

// TestAdaptForwards checks the adapter that lets a filter at another width be
// measured. It has to forward RealCoefficients, or a float32 filter would
// silently lose the exact path, and Ratio, or a rate changer would be measured
// at the wrong output rate.
func TestAdaptForwards(t *testing.T) {
	const rate = 48000.0
	f32 := must(dsp.NewIIRFilter([]float32{0.5, 0.5}, []float32{1, -0.5}))
	p := Adapt(f32)
	b, a, ok := realCoefficients(p)
	wantB, wantA, _ := f32.RealCoefficients()
	if !ok || !slices.Equal(b, wantB) || !slices.Equal(a, wantA) {
		t.Fatalf("coefficients (%v, %v, %v), want (%v, %v, true)", b, a, ok, wantB, wantA)
	}
	r, err := FreqResponse(p, rate, LogGrid(20, 20000, 64), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != MethodExact {
		t.Errorf("measured by %v, want the exact method", r.Method)
	}
	if _, out := rates(p, rate); out != rate {
		t.Errorf("output rate %v, want %v", out, rate)
	}

	// The samples go through at the filter's width, and Reset reaches it.
	in := []float64{1, 0.25, -0.5, 0}
	first := run(p, nil, in)
	again := run(p, nil, in)
	want := make([]float32, len(in))
	must(dsp.NewIIRFilter([]float32{0.5, 0.5}, []float32{1, -0.5})).Process(want, []float32{1, 0.25, -0.5, 0})
	for i := range want {
		if first[i] != float64(want[i]) || again[i] != first[i] {
			t.Fatalf("[%d] = %v then %v, want %v", i, first[i], again[i], want[i])
		}
	}

	// A complex decimator: Ratio forwards, and the real part comes back.
	d := Adapt(must(dsp.NewFIRDecimator([]complex64{1, 1}, 2)))
	if got := Phases(d); got != 2 {
		t.Errorf("Phases = %d, want 2", got)
	}
	if _, out := rates(d, rate); out != rate/2 {
		t.Errorf("decimator output rate %v, want %v", out, rate/2)
	}
	if _, _, ok := realCoefficients(d); ok {
		t.Error("a rate changer must not report a transfer function")
	}
	if got := run(d, nil, []float64{1, 2, 3, 4}); !slices.Equal(got, []float64{1, 5}) {
		t.Errorf("decimated %v, want [1 5]", got)
	}

	// Adapting a float64 processor is the processor itself.
	f64 := must(dsp.NewFIRFilter([]float64{1}))
	if Adapt(f64) != dsp.Processor[float64, float64](f64) {
		t.Error("Adapt wrapped a float64 processor")
	}
}
