package dspviz

import (
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

// TestPalettesByName checks the name-to-palette lookup the command line and
// the server both drive. Every name PaletteNames advertises has to resolve, and
// each one has to be a different picture: an unknown name that returned viridis
// would draw a plausible chart that is the wrong one.
func TestPalettesByName(t *testing.T) {
	names := PaletteNames()
	if len(names) == 0 {
		t.Fatal("PaletteNames is empty")
	}
	if slices.Contains(names, "grey") {
		t.Error("PaletteNames lists the alternative spelling of gray, which its doc says it does not")
	}

	seen := make(map[color.RGBA]string, len(names))
	for _, name := range names {
		p, ok := PaletteByName(name)
		if !ok {
			t.Errorf("%s: PaletteByName does not know a name PaletteNames advertises", name)
			continue
		}
		// A palette has to map the whole range, and the two ends have to
		// differ, or a spectrogram drawn with it is one flat color.
		lo, hi := p(0), p(1)
		if lo == hi {
			t.Errorf("%s: both ends of the ramp are %v", name, lo)
		}
		if lo.A != 0xff || hi.A != 0xff {
			t.Errorf("%s: not opaque: %v, %v", name, lo, hi)
		}
		// Out of range is clamped rather than extrapolated or wrapped.
		if p(-5) != lo || p(5) != hi {
			t.Errorf("%s: out-of-range values are not clamped", name)
		}
		if other, dup := seen[hi]; dup {
			t.Errorf("%s ends the same color as %s", name, other)
		}
		seen[hi] = name
	}

	// The alternative spelling resolves even though it is not advertised.
	if _, ok := PaletteByName("grey"); !ok {
		t.Error(`PaletteByName("grey") should resolve`)
	}
	if _, ok := PaletteByName("nonesuch"); ok {
		t.Error("PaletteByName accepted a name that is not a palette")
	}
}

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
	p, err := NewFuncProcessor(func(dst, src []float64) []float64 {
		for _, x := range src {
			dst = append(dst, math.Round(x*steps)/steps)
		}
		return dst
	}, nil, rate, rate)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ToneSpectrum(p, 1000, SpectrumOptions{Frame: 1 << 13})
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
	p, err := NewFIRProcessor(taps, rate)
	if err != nil {
		t.Fatal(err)
	}
	r, err := FreqResponse(p, LinearGrid(100, 6000, 64), ResponseOptions{})
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
	p, err := NewFIRProcessor(taps, rate)
	if err != nil {
		t.Fatal(err)
	}

	fine, err := FreqResponse(p, LinearGrid(100, 6000, 512), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fine.Method != MethodExact {
		t.Fatalf("measured by %v, so the delay is differenced and this test proves nothing", fine.Method)
	}
	if w := fine.GridWarning(); w != "" {
		t.Errorf("a grid stepping 11.5 Hz against a 200-sample delay warned: %s", w)
	}

	coarse, err := FreqResponse(p, LinearGrid(100, 20000, 8), ResponseOptions{})
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
	const rate = 48000.0
	taps := []float64{0.25, 0.5, 0.25}
	p, err := NewFIRProcessor(taps, rate)
	if err != nil {
		t.Fatal(err)
	}

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

// TestKindByNameCoversTheCatalog checks the lookup against the catalog itself,
// so an entry added to one and not reachable through the other cannot pass.
func TestKindByNameCoversTheCatalog(t *testing.T) {
	for _, k := range Catalog() {
		got, ok := KindByName(k.Name)
		if !ok {
			t.Errorf("%s: KindByName does not find a catalog entry", k.Name)
			continue
		}
		if got.Label != k.Label || len(got.Params) != len(k.Params) {
			t.Errorf("%s: KindByName returned a different entry", k.Name)
		}
	}
	if _, ok := KindByName("nonesuch"); ok {
		t.Error("KindByName accepted a name that is not in the catalog")
	}
	if _, ok := KindByName(""); ok {
		t.Error("KindByName accepted an empty name")
	}
}

// TestIIRProcessor covers the adapter for dsp.IIRFilter, which nothing else
// reaches: the catalog has no entry that builds one, so the only thing standing
// behind it is that its Coefficients pass through unchanged and its response
// matches dsp.Freqz.
func TestIIRProcessor(t *testing.T) {
	const rate = 48000.0
	f := must(dsp.NewIIRFilter([]float64{0.5, 0.5}, []float64{1, -0.5}))
	p, err := NewIIRProcessor(f, rate)
	if err != nil {
		t.Fatal(err)
	}
	if in, out := p.Rates(); in != rate || out != rate {
		t.Errorf("rates (%v, %v), want (%v, %v)", in, out, rate, rate)
	}
	c, ok := p.(Coefficients)
	if !ok {
		t.Fatal("an IIR processor does not report its coefficients, so it cannot be measured exactly")
	}
	b, a := c.Coefficients()
	wantB, wantA := f.Coefficients()
	if !slices.Equal(b, wantB) || !slices.Equal(a, wantA) {
		t.Errorf("coefficients (%v, %v), want (%v, %v)", b, a, wantB, wantA)
	}

	r, err := FreqResponse(p, LogGrid(20, 20000, 64), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != MethodExact {
		t.Errorf("measured by %v, want the exact method", r.Method)
	}
	// Processing has to work too, and the filter has to be reset between runs.
	first := p.Process(nil, []float64{1, 0, 0, 0})
	p.Reset()
	again := p.Process(nil, []float64{1, 0, 0, 0})
	if !slices.Equal(first, again) {
		t.Errorf("after Reset got %v, want %v", again, first)
	}

	if _, err := NewIIRProcessor(nil, rate); err == nil {
		t.Error("a nil filter should be an error")
	}
	if _, err := NewIIRProcessor(f, 0); err == nil {
		t.Error("a zero rate should be an error")
	}
}

// TestReportWriteSummary covers the text the command line prints, which is the
// only reader of most of Metrics. It is written line by line from optional
// fields, so a report missing a section has to print the rest rather than a
// row of zeros.
func TestReportWriteSummary(t *testing.T) {
	const rate = 48000.0
	p, err := NewBiQuadProcessor(must(dsp.NewLowPassBiQuad[float64](rate, 1000, 0.7071)), rate)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(p, ReportOptions{
		Points: 128, Frame: 4096, ToneHz: 997, SweepSec: -1,
		Passband: [2]float64{20, 500}, Stopband: [2]float64{4000, 20000},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := rep.WriteSummary(&buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"rate          48000 Hz in, 48000 Hz out",
		"method        ",
		"cutoff        ",
		"passband      20-500 Hz",
		"stopband      4000-20000 Hz",
		"group delay   ",
		// The tone is snapped to an odd bin, so it is near 997 rather than at
		// it; that is CoherentFreq's doing and the summary reports where the
		// measurement was actually made.
		"SFDR ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary omits %q:\n%s", want, got)
		}
	}
	// Every line has to carry a number rather than a NaN or an infinity, which
	// is what an empty band or a silent tone would leave behind.
	for _, bad := range []string{"NaN", "Inf"} {
		if strings.Contains(got, bad) {
			t.Errorf("the summary holds %s:\n%s", bad, got)
		}
	}

	// A report with no tone measurement and no bands prints the rest.
	bare := &Report{Rates: [2]float64{rate, rate}, Response: rep.Response, Warnings: []string{"a note"}}
	buf.Reset()
	if err := bare.WriteSummary(&buf); err != nil {
		t.Fatal(err)
	}
	got = buf.String()
	// The column spacing is part of the match: "passband" also appears in the
	// group delay line, which every report has.
	if strings.Contains(got, "passband  ") || strings.Contains(got, "SFDR") {
		t.Errorf("a bare report printed a section it has no numbers for:\n%s", got)
	}
	if !strings.Contains(got, "note          a note") {
		t.Errorf("the warning did not print:\n%s", got)
	}
}
