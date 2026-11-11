package analysis

import (
	"math"
	"strings"
	"testing"
)

func statsOf(t *testing.T, x []float64, opt StatsOptions) *SignalStats {
	t.Helper()
	s, err := StatsOf(x, opt)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// closedStats closes a builder for a test that already wrote to it.
func closedStats(t *testing.T, b *StatsBuilder) *SignalStats {
	t.Helper()
	s, err := b.Close()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestStatsClosedForms checks the accumulators against the arithmetic they are
// supposed to be doing: for a sine of amplitude a the RMS is a/sqrt(2), the
// crest factor is sqrt(2) and the offset is zero, all exactly.
func TestStatsClosedForms(t *testing.T) {
	const (
		rate = 8000.0
		amp  = 0.5
		n    = 8000 // 800 Hz, a whole ten samples per cycle
	)
	x := make([]float64, n)
	for i := range x {
		x[i] = amp * math.Cos(2*math.Pi*float64((100*i)%1000)/1000)
	}
	s := statsOf(t, x, StatsOptions{Rate: rate})

	if math.Abs(s.RMS-amp/math.Sqrt2) > 1e-12 {
		t.Errorf("rms %v, want %v", s.RMS, amp/math.Sqrt2)
	}
	if math.Abs(s.Peak-amp) > 1e-12 {
		t.Errorf("peak %v, want %v", s.Peak, amp)
	}
	if math.Abs(s.Crest-math.Sqrt2) > 1e-12 {
		t.Errorf("crest %v, want %v", s.Crest, math.Sqrt2)
	}
	if math.Abs(s.DC) > 1e-12 {
		t.Errorf("dc %v, want 0", s.DC)
	}
	if math.Abs(s.Duration-1) > 1e-12 {
		t.Errorf("duration %v, want 1", s.Duration)
	}
	// An 800 Hz sine crosses zero twice per cycle.
	if math.Abs(s.ZeroCrossRateHz-1600) > 1 {
		t.Errorf("zero crossings %v per second, want 1600", s.ZeroCrossRateHz)
	}
}

// TestStatsEffectiveBits is the heuristic that explains a noise floor 24 dB
// above expectation in one line: an eight-bit signal in a sixteen-bit container
// leaves the low eight bits of every code zero.
func TestStatsEffectiveBits(t *testing.T) {
	const bits = 16
	full := math.Ldexp(1, bits-1)

	native := make([]float64, 4000)
	shifted := make([]float64, 4000)
	for i := range native {
		code := math.Round(0.4 * full * math.Sin(float64(i)*0.013))
		native[i] = code / full
		// The same signal written as eight bits and left-justified into sixteen.
		shifted[i] = math.Round(code/256) * 256 / full
	}
	if got := statsOf(t, native, StatsOptions{Rate: 8000, Bits: bits}).EffectiveBits; got != bits {
		t.Errorf("a full-depth signal reports %d bits, want %d", got, bits)
	}
	s := statsOf(t, shifted, StatsOptions{Rate: 8000, Bits: bits})
	if s.EffectiveBits != 8 {
		t.Errorf("an 8-bit signal in a 16-bit container reports %d bits, want 8", s.EffectiveBits)
	}
	if s.BitsNote == "" {
		t.Error("a short signal should say so in a note")
	}

	// A float format has no codes to count, and neither does a signal that has
	// been off the code grid: the estimate has to decline rather than guess.
	if got := statsOf(t, native, StatsOptions{Rate: 8000}); got.EffectiveBits != 0 || got.BitsNote == "" {
		t.Errorf("a float signal reports %d bits with note %q", got.EffectiveBits, got.BitsNote)
	}
	off := make([]float64, len(native))
	for i := range off {
		off[i] = native[i] * 0.37
	}
	if got := statsOf(t, off, StatsOptions{Rate: 8000, Bits: bits}); got.EffectiveBits != 0 {
		t.Errorf("a scaled signal reports %d bits; it is not on the code grid at all", got.EffectiveBits)
	}
}

func TestStatsSilence(t *testing.T) {
	const rate = 1000.0
	x := make([]float64, 3000)
	for i := range x {
		if i >= 1000 && i < 2500 {
			continue // a second and a half of digital silence
		}
		x[i] = 0.5 * math.Sin(float64(i))
	}
	s := statsOf(t, x, StatsOptions{Rate: rate})
	if math.Abs(s.SilentFraction-0.5) > 0.05 {
		t.Errorf("silent fraction %v, want about 0.5", s.SilentFraction)
	}
	if math.Abs(s.LongestSilence-1.5) > 0.05 {
		t.Errorf("longest silence %v s, want about 1.5", s.LongestSilence)
	}
}

// TestIQImbalanceRecovers injects exactly known defects and requires them back.
func TestIQImbalanceRecovers(t *testing.T) {
	const (
		rate     = 48000.0
		n        = 48000
		gainDB   = 0.5
		quadDeg  = 3.0
		toneCycl = 1000
	)
	gain := Amplitude(-gainDB) // applied to Q, so I sits gainDB above it
	phi := quadDeg * math.Pi / 180

	z := make([]complex128, n)
	clean := make([]complex128, n)
	for i := range z {
		th := 2 * math.Pi * float64((toneCycl*i)%n) / n
		clean[i] = complex(math.Cos(th), math.Sin(th))
		z[i] = complex(math.Cos(th), gain*math.Sin(th+phi))
	}

	b, err := NewStatsBuilder(StatsOptions{Rate: rate, Complex: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteComplex(z); err != nil {
		t.Fatal(err)
	}
	q := closedStats(t, b).IQ
	if q == nil {
		t.Fatal("no I/Q statistics")
	}
	if math.Abs(q.GainImbalanceDB-gainDB) > 0.01 {
		t.Errorf("gain imbalance %v dB, want %v", q.GainImbalanceDB, gainDB)
	}
	if math.Abs(q.QuadratureErrorDeg-quadDeg) > 0.05 {
		t.Errorf("quadrature error %v degrees, want %v", q.QuadratureErrorDeg, quadDeg)
	}

	cb, err := NewStatsBuilder(StatsOptions{Rate: rate, Complex: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.WriteComplex(clean); err != nil {
		t.Fatal(err)
	}
	c := closedStats(t, cb).IQ
	if math.Abs(c.GainImbalanceDB) > 1e-9 || math.Abs(c.QuadratureErrorDeg) > 1e-9 {
		t.Errorf("a clean signal reports %v dB and %v degrees, want zero",
			c.GainImbalanceDB, c.QuadratureErrorDeg)
	}
}

// TestIQStatsFindDualMono is the same three sums read as a stereo question: two
// identical channels are dual mono, and two a quarter cycle apart are an I/Q
// pair someone wrote as left and right.
func TestIQStatsFindDualMono(t *testing.T) {
	const n = 4096
	same := make([]complex128, n)
	quad := make([]complex128, n)
	for i := range same {
		th := 2 * math.Pi * float64((7*i)%n) / n
		same[i] = complex(math.Cos(th), math.Cos(th))
		quad[i] = complex(math.Cos(th), math.Sin(th))
	}
	for _, tt := range []struct {
		name    string
		x       []complex128
		dual    bool
		quadDeg float64
	}{
		{"dual mono", same, true, 90},
		{"quadrature", quad, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewStatsBuilder(StatsOptions{Rate: 8000, Complex: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.WriteComplex(tt.x); err != nil {
				t.Fatal(err)
			}
			q := closedStats(t, b).IQ
			if q.DualMono != tt.dual {
				t.Errorf("dual mono %v, want %v (correlation %v)", q.DualMono, tt.dual, q.Correlation)
			}
			if math.Abs(math.Abs(q.QuadratureErrorDeg)-tt.quadDeg) > 0.01 {
				t.Errorf("quadrature error %v degrees, want %v", q.QuadratureErrorDeg, tt.quadDeg)
			}
		})
	}
}

// TestSpectralStatsSpotsARealSignalReadAsIQ is one of the checks that costs
// nothing once the spectrum exists: a two-sided spectrum that is its own mirror
// means the samples are not complex at all.
func TestSpectralStatsSpotsARealSignalReadAsIQ(t *testing.T) {
	const (
		size = 512
		rate = 8000.0
	)
	x := make([]complex128, 32*size)
	for i := range x {
		v := math.Cos(2 * math.Pi * float64((37*i)%size) / size)
		x[i] = complex(v, 0) // a real signal handed over as I/Q
	}
	p, err := PSDOfComplex(x, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	s := SpectralStatsOf(p)
	if len(s.Notes) == 0 || !strings.Contains(s.Notes[0], "really a real signal") {
		t.Errorf("a conjugate-symmetric spectrum went unreported: %v", s.Notes)
	}

	// A genuinely complex signal is not its own mirror and must not be reported
	// as one.
	for i := range x {
		th := 2 * math.Pi * float64((37*i)%size) / size
		x[i] = complex(math.Cos(th), math.Sin(th))
	}
	p, err = PSDOfComplex(x, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	if s := SpectralStatsOf(p); len(s.Notes) != 0 {
		t.Errorf("a complex signal was reported as real: %v", s.Notes)
	}
	if s := SpectralStatsOf(p); s.ImageRejectionDB > -50 {
		t.Errorf("image rejection %v dB, want it far down", s.ImageRejectionDB)
	}
}
