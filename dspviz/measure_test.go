package dspviz

import (
	"math"
	"math/cmplx"
	"testing"

	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/firpm"
)

// TestMeasurementNoiseFloor checks that a coherent tone read back over a whole
// number of its own cycles leaks nothing, so a perfectly transparent processor
// has to come back with the tone and nothing else -- and "nothing else" means
// below the arithmetic, hundreds of decibels down, not below some window's sidelobes.
//
// If this ever fails, no stopband measurement anywhere in the package means
// what it says.
func TestMeasurementNoiseFloor(t *testing.T) {
	const rate, frame = 48000.0, 1 << 16
	p, err := Identity(rate)
	if err != nil {
		t.Fatal(err)
	}
	_, k := CoherentFreq(997, rate, frame)

	in := Tone(make([]float64, frame), k, frame, 1)
	out := p.Process(nil, in)

	signal := cmplx.Abs(dsp.DFTBinReal(out, k))
	var worst float64
	var worstBin int
	for j := 1; j < frame/2; j++ {
		if j == k {
			continue
		}
		if m := cmplx.Abs(dsp.DFTBinReal(out, j)); m > worst {
			worst, worstBin = m, j
		}
		if j > 400 { // the whole sweep is O(n^2); a few hundred bins is plenty
			break
		}
	}
	db := 20 * math.Log10(worst/signal)
	t.Logf("worst non-signal bin %d at %.1f dB", worstBin, db)
	if db > -280 {
		t.Errorf("measurement floor is %.1f dB, want below -280", db)
	}
}

// TestNoiseFloorNeedsExactPhase is the other half of the same argument. The
// obvious way to write a tone generator, with the phase argument computed in
// floating point, loses the low bits of a large argument in a patterned way,
// and the result is spurious tones far above the arithmetic floor. This test
// pins the size of that gap, so nobody later "simplifies" Tone back into the
// broken form and finds every plot quietly gaining a -200 dB noise floor.
func TestNoiseFloorNeedsExactPhase(t *testing.T) {
	const rate, frame = 48000.0, 1 << 16
	// High in the band, where the phase argument grows largest and the gap is
	// widest. Near DC the two forms are only about 20 dB apart.
	hz, k := CoherentFreq(23000, rate, frame)

	naive := make([]float64, frame)
	for i := range naive {
		naive[i] = math.Cos(2 * math.Pi * hz * float64(i) / rate)
	}
	exact := Tone(make([]float64, frame), k, frame, 1)

	floor := func(x []float64) float64 {
		signal := cmplx.Abs(dsp.DFTBinReal(x, k))
		var worst float64
		for j := 1; j < 400; j++ {
			if j == k {
				continue
			}
			worst = math.Max(worst, cmplx.Abs(dsp.DFTBinReal(x, j)))
		}
		return 20 * math.Log10(worst/signal)
	}

	naiveDB, exactDB := floor(naive), floor(exact)
	t.Logf("naive phase argument: %.1f dB, exact: %.1f dB, gap %.1f dB", naiveDB, exactDB, naiveDB-exactDB)
	if exactDB > -300 {
		t.Errorf("the exact generator leaks %.1f dB, want below -300", exactDB)
	}
	if naiveDB < -290 {
		t.Errorf("the naive generator leaks only %.1f dB, better than the -259 dB Tone's comment quotes; "+
			"if the arithmetic improved, the comment needs revisiting", naiveDB)
	}
	if naiveDB-exactDB < 50 {
		t.Errorf("the two generators are only %.1f dB apart; Tone's comment claims 20 to 115 dB "+
			"and this case should be near the top of that, so either the claim or this test is now wrong",
			naiveDB-exactDB)
	}
}

// TestToneMatchesFreqz checks the black-box path against the exact one. They
// are different computations over different quantities -- one runs a signal
// through the filter and correlates, the other evaluates a polynomial ratio --
// so agreeing to a part in 10^10 means both are right.
func TestToneMatchesFreqz(t *testing.T) {
	const rate = 48000.0
	for _, c := range []struct {
		name string
		make func() Processor
	}{
		{"lowpass", func() Processor {
			p, _ := NewBiQuadProcessor(must(dsp.NewLowPassBiQuad[float64](rate, 1000, 0.7071)), rate)
			return p
		}},
		{"peaking", func() Processor {
			p, _ := NewBiQuadProcessor(must(dsp.NewPeakingEQBiQuad[float64](rate, 3000, 4, 12)), rate)
			return p
		}},
		{"fir", func() Processor {
			taps, _, err := firpm.Design(firpm.Spec{
				NumTaps: 41, SampleRate: rate,
				Bands: []firpm.Band{{Lower: 0, Upper: 4000, Response: 1, Weight: 1},
					{Lower: 8000, Upper: 24000, Response: 0, Weight: 1}},
			})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := NewFIRProcessor(taps, rate)
			return p
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			freq := LinearGrid(200, 20000, 40)
			// The tone path snaps each frequency onto a bin, so measure the
			// frequencies it actually used rather than the ones asked for.
			tone, err := FreqResponse(c.make(), freq, ResponseOptions{Method: MethodTone, Frame: 1 << 14})
			if err != nil {
				t.Fatal(err)
			}
			exact, err := FreqResponse(c.make(), tone.Freq, ResponseOptions{Method: MethodExact})
			if err != nil {
				t.Fatal(err)
			}
			for i := range tone.H {
				if d := cmplx.Abs(tone.H[i] - exact.H[i]); d > 1e-10 {
					t.Fatalf("%.1f Hz: tone %v, exact %v (differ by %g)",
						tone.Freq[i], tone.H[i], exact.H[i], d)
				}
			}
		})
	}
}

// TestImpulseMethodMatchesExactForFIR checks that transforming a truncated
// impulse response is the same computation as evaluating the transfer function,
// when the truncation throws nothing away. For an FIR filter it should be exact
// to the arithmetic; the point of calling MethodImpulse a survey is that
// this stops being true the moment there is feedback.
func TestImpulseMethodMatchesExactForFIR(t *testing.T) {
	const rate = 48000.0
	taps := []float64{0.02, -0.05, 0.11, 0.37, 0.37, 0.11, -0.05, 0.02}
	mk := func() Processor { p, _ := NewFIRProcessor(taps, rate); return p }

	freq := LogGrid(20, 20000, 64)
	exact, err := FreqResponse(mk(), freq, ResponseOptions{Method: MethodExact})
	if err != nil {
		t.Fatal(err)
	}
	imp, err := FreqResponse(mk(), freq, ResponseOptions{Method: MethodImpulse, Frame: 64})
	if err != nil {
		t.Fatal(err)
	}
	for i := range freq {
		if d := cmplx.Abs(imp.H[i] - exact.H[i]); d > 1e-14 {
			t.Fatalf("%.1f Hz: impulse %v, exact %v", freq[i], imp.H[i], exact.H[i])
		}
	}
	if len(imp.Warnings) == 0 {
		t.Error("MethodImpulse should warn that it truncated")
	}
}

// TestFIRPMRippleMatchesDeviation is the strongest test here. firpm reports
// the weighted Chebyshev error it achieved, but nothing in this module has
// ever been able to check that claim -- firpm's own doc comment tells the
// caller to confirm it with an FFT that did not exist. Measuring the ripple
// and the stopband peak independently and finding them where the deviation
// says they should be validates firpm's number, dsp.Freqz, and the band
// metrics at once.
//
// The tolerance follows firpm's documented grid-density bound: the response
// overshoots the reported deviation by up to about 3% at density 16 and 0.1%
// at 32. That the agreement tightens by the predicted factor as the density
// doubles is itself the evidence that neither side is measuring the other.
func TestFIRPMRippleMatchesDeviation(t *testing.T) {
	const rate = 48000.0
	for _, c := range []struct {
		name     string
		taps     int
		bands    []firpm.Band
		density  int
		tolerate float64
	}{
		{"lowpass/16", 63, []firpm.Band{
			{Lower: 0, Upper: 6000, Response: 1, Weight: 1},
			{Lower: 9000, Upper: 24000, Response: 0, Weight: 1}}, 16, 0.03},
		{"lowpass/32", 63, []firpm.Band{
			{Lower: 0, Upper: 6000, Response: 1, Weight: 1},
			{Lower: 9000, Upper: 24000, Response: 0, Weight: 1}}, 32, 0.01},
		{"lowpass/128", 63, []firpm.Band{
			{Lower: 0, Upper: 6000, Response: 1, Weight: 1},
			{Lower: 9000, Upper: 24000, Response: 0, Weight: 1}}, 128, 0.001},
		{"weighted/32", 81, []firpm.Band{
			{Lower: 0, Upper: 5000, Response: 1, Weight: 1},
			{Lower: 7000, Upper: 24000, Response: 0, Weight: 10}}, 32, 0.01},
		{"bandpass/32", 101, []firpm.Band{
			{Lower: 0, Upper: 3000, Response: 0, Weight: 1},
			{Lower: 6000, Upper: 10000, Response: 1, Weight: 1},
			{Lower: 13000, Upper: 24000, Response: 0, Weight: 1}}, 32, 0.02},
	} {
		t.Run(c.name, func(t *testing.T) {
			taps, dev, err := firpm.Design(firpm.Spec{
				NumTaps: c.taps, SampleRate: rate, Bands: c.bands, GridDensity: c.density,
			})
			if err != nil {
				t.Fatal(err)
			}
			p, err := NewFIRProcessor(taps, rate)
			if err != nil {
				t.Fatal(err)
			}

			for _, band := range c.bands {
				// Sample the band densely enough that the extremal ripples,
				// which are what the deviation describes, are actually hit.
				freq := LinearGrid(band.Lower, band.Upper, 20001)
				r, err := FreqResponse(p, freq, ResponseOptions{Method: MethodExact})
				if err != nil {
					t.Fatal(err)
				}
				want := dev / band.Weight

				var worst float64
				for _, m := range r.Magnitude() {
					worst = math.Max(worst, math.Abs(m-band.Response))
				}
				// The deviation is what the exchange achieved on its own
				// discrete grid, so the continuous response can overshoot it
				// but must never fall short: a measured ripple below the
				// reported deviation would mean one of the two is wrong.
				rel := (worst - want) / want
				t.Logf("band %g-%g Hz: overshoot %.4f%%", band.Lower, band.Upper, rel*100)
				if rel < -1e-9 {
					t.Errorf("band %g-%g Hz: peak error %g is below the reported deviation %g",
						band.Lower, band.Upper, worst, want)
				}
				if rel > c.tolerate {
					t.Errorf("band %g-%g Hz: peak error %g overshoots the reported deviation %g by %.4f, allowed %.4f",
						band.Lower, band.Upper, worst, want, rel, c.tolerate)
				}
			}
		})
	}
}

// TestGroupDelayOfLinearPhaseFIR checks the measured group delay against the
// value symmetry guarantees, through the same path a plot would use.
func TestGroupDelayOfLinearPhaseFIR(t *testing.T) {
	const rate = 48000.0
	taps, _, err := firpm.Design(firpm.Spec{
		NumTaps: 63, SampleRate: rate,
		Bands: []firpm.Band{{Lower: 0, Upper: 6000, Response: 1, Weight: 1},
			{Lower: 9000, Upper: 24000, Response: 0, Weight: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := NewFIRProcessor(taps, rate)
	r, err := FreqResponse(p, LinearGrid(100, 5500, 2001), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := float64(len(taps)-1) / 2
	d := r.GroupDelay()
	for i := 1; i < len(d)-1; i++ {
		if math.Abs(d[i]-want) > 1e-6 {
			t.Fatalf("%.1f Hz: group delay %v, want %v", r.Freq[i], d[i], want)
		}
	}

	// And the residual phase, which is what the phase plot shows, has to be
	// flat: a linear-phase filter is a pure delay in its passband.
	for i, v := range r.ResidualPhase() {
		if math.Abs(v) > 1e-6 {
			t.Fatalf("%.1f Hz: residual phase %v degrees, want 0", r.Freq[i], v)
		}
	}
	m := Measure(r, [2]float64{100, 5500}, [2]float64{})
	if math.Abs(m.GroupDelaySamples-want) > 1e-6 {
		t.Errorf("Measure reported group delay %v, want %v", m.GroupDelaySamples, want)
	}
}

// TestBiQuadCutoffMetric checks that the reported cutoff is interpolated rather
// than snapped to a grid point, by asking for it on a grid far too coarse to
// contain the answer.
func TestBiQuadCutoffMetric(t *testing.T) {
	const rate, cutoff = 48000.0, 1000.0
	p, err := NewBiQuadProcessor(must(dsp.NewLowPassBiQuad[float64](rate, cutoff, math.Sqrt2/2)), rate)
	if err != nil {
		t.Fatal(err)
	}
	r, err := FreqResponse(p, LogGrid(20, 20000, 200), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := Measure(r, [2]float64{20, 200}, [2]float64{8000, 20000})
	if math.Abs(m.CutoffHz-cutoff) > 1 {
		t.Errorf("cutoff %v Hz, want %v", m.CutoffHz, cutoff)
	}
	if m.Passband.RippleDB > 0.2 {
		t.Errorf("passband ripple %v dB, want a flat Butterworth passband", m.Passband.RippleDB)
	}
	// A two-pole low-pass falls 12 dB per octave, so at 8 kHz -- three octaves
	// above the 1 kHz corner -- it is down about 36 dB, and that is what the
	// stopband peak has to be rather than some rounder number.
	if want := -36.0; math.Abs(m.Stopband.MaxDB-want) > 3 {
		t.Errorf("stopband peak %v dB, want about %v (12 dB per octave, three octaves up)",
			m.Stopband.MaxDB, want)
	}
}
