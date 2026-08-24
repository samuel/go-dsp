package dspviz

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
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

// TestStatsClipping is the trap a naive counter falls into: a peak-normalized
// file has a sample on the rail by construction, and reporting that as clipping
// makes the number useless.
func TestStatsClipping(t *testing.T) {
	const bits = 16
	full := math.Ldexp(1, bits-1)
	rail := (full - 1) / full

	// Seven samples on the rail in three runs, of four, two and one.
	x := make([]float64, 200)
	for i := range x {
		x[i] = 0.25
	}
	for _, r := range [][2]int{{10, 4}, {50, 2}, {90, 1}} {
		for i := range r[1] {
			x[r[0]+i] = rail
		}
	}
	s := statsOf(t, x, StatsOptions{Rate: 8000, Bits: bits})
	if s.Clipped != 7 {
		t.Errorf("%d samples on the rail, want 7", s.Clipped)
	}
	if s.ClippedRuns != 3 {
		t.Errorf("%d runs, want 3", s.ClippedRuns)
	}
	if s.SustainedRuns != 1 {
		t.Errorf("%d sustained runs, want 1 (only the run of four is longer than %d)", s.SustainedRuns, minClipRun)
	}

	// A peak-normalized file: one sample at the rail and nothing else. It is not
	// clipped, and the summary must not say it is.
	y := make([]float64, 200)
	for i := range y {
		y[i] = 0.5 * math.Sin(float64(i))
	}
	y[7] = rail
	p := statsOf(t, y, StatsOptions{Rate: 8000, Bits: bits})
	if p.SustainedRuns != 0 {
		t.Errorf("a peak-normalized file reports %d sustained runs, want 0", p.SustainedRuns)
	}
	var buf bytes.Buffer
	if err := p.WriteSummary(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "clipping      none") {
		t.Errorf("the summary does not say the file is unclipped:\n%s", buf.String())
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

// TestStatsJSONSurvivesSilence checks that WriteJSON can serialize a
// stats document for an all-zero signal, whose -Inf PeakDB that
// encoding/json refuses to emit.
func TestStatsJSONSurvivesSilence(t *testing.T) {
	s := statsOf(t, make([]float64, 1000), StatsOptions{Rate: 8000})
	if !math.IsInf(s.PeakDB, -1) {
		t.Fatalf("silence peaks at %v dBFS, want -Inf", s.PeakDB)
	}
	if _, err := json.Marshal(s); err == nil {
		t.Fatal("encoding/json accepted an infinity; this test no longer proves anything")
	}
	var buf bytes.Buffer
	if err := s.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if m["peakDB"].(float64) >= 0 {
		t.Errorf("peakDB came out as %v, want it pinned to the bottom of the range", m["peakDB"])
	}
}

// TestStatsJSONCoversEveryField checks that every field SignalStats declares
// reaches the JSON.
//
// It walks the tags and looks for each one in the encoded output, so a field
// the struct carries but the document skips cannot pass.
func TestStatsJSONCoversEveryField(t *testing.T) {
	s := &SignalStats{
		IQ:       &IQStats{},
		Spectral: &SpectralStats{Notes: []string{"a spectral note"}},
		Notes:    []string{"a note"},
		BitsNote: "a bits note",
	}
	var buf bytes.Buffer
	if err := s.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}

	rt := reflect.TypeOf(*s)
	for f := range rt.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		if name == "complex" {
			continue // omitempty, and this fixture is a real signal
		}
		if _, ok := got[name]; !ok {
			t.Errorf("the JSON is missing %q, which SignalStats declares", name)
		}
	}

	sp, ok := got["spectral"].(map[string]any)
	if !ok {
		t.Fatalf("no spectral section: %v", got["spectral"])
	}
	if _, ok := sp["noiseFloorDB"]; !ok {
		t.Errorf("the spectral section is missing its own fields: %v", sp)
	}
	if n, _ := sp["notes"].([]any); len(n) != 1 {
		t.Errorf("spectral notes = %v, want one", sp["notes"])
	}
	if n, _ := got["notes"].([]any); len(n) != 1 {
		t.Errorf("notes = %v, want one", got["notes"])
	}
}

// TestReportJSONCarriesPairNotes checks the other document, whose pair entries
// were hand-built the same way and dropped their notes.
func TestReportJSONCarriesPairNotes(t *testing.T) {
	r := &SignalReport{
		Channels: []*SignalStats{{Rate: 8000}},
		Pairs:    []*ChannelPairStats{{A: 0, B: 1, Notes: []string{"a pair note"}}},
		Notes:    []string{"a report note"},
	}
	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	pairs, _ := got["pairs"].([]any)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %v, want one", got["pairs"])
	}
	p, _ := pairs[0].(map[string]any)
	if n, _ := p["notes"].([]any); len(n) != 1 {
		t.Errorf("pair notes = %v, want one", p["notes"])
	}
}
