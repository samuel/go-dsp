package analysis

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// pairOf accumulates two channels and closes the comparison.
func pairOf(t *testing.T, a, b []float64) *ChannelPairStats {
	t.Helper()
	p := NewPairBuilder(0, 1)
	if err := p.Write(a, b); err != nil {
		t.Fatal(err)
	}
	return closedPair(t, p)
}

// closedPair closes a builder for a test that already wrote to it.
func closedPair(t *testing.T, p *PairBuilder) *ChannelPairStats {
	t.Helper()
	s, err := p.Close()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestPairClosedForms checks the three numbers against cases whose answers are
// arithmetic rather than opinion.
func TestPairClosedForms(t *testing.T) {
	const n = 4096
	tone := make([]float64, n)
	other := make([]float64, n)
	half := make([]float64, n)
	inv := make([]float64, n)
	quad := make([]float64, n)
	for i := range tone {
		th := 2 * math.Pi * float64((7*i)%n) / n
		tone[i] = 0.5 * math.Cos(th)
		other[i] = 0.5 * math.Cos(2*math.Pi*float64((11*i)%n)/n)
		half[i] = tone[i] / 2
		inv[i] = -tone[i]
		quad[i] = 0.5 * math.Sin(th)
	}

	for _, tc := range []struct {
		name     string
		a, b     []float64
		corr     float64
		balance  float64
		dual     bool
		inverted bool
	}{
		{"identical", tone, tone, 1, 0, true, false},
		{"inverted", tone, inv, -1, 0, false, true},
		// Half the level is neither a duplicate nor an inversion, however well
		// it correlates -- which is why the verdicts test the balance too.
		{"half level", tone, half, 1, 6.0206, false, false},
		// Two different bins of the same transform are orthogonal over a whole
		// number of cycles, so the correlation is exactly zero.
		{"different tones", tone, other, 0, 0, false, false},
		{"quadrature", tone, quad, 0, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := pairOf(t, tc.a, tc.b)
			if math.Abs(s.Correlation-tc.corr) > 1e-9 {
				t.Errorf("correlation %v, want %v", s.Correlation, tc.corr)
			}
			if math.Abs(s.BalanceDB-tc.balance) > 1e-4 {
				t.Errorf("balance %v dB, want %v", s.BalanceDB, tc.balance)
			}
			if s.DualMono != tc.dual {
				t.Errorf("dual mono %v, want %v", s.DualMono, tc.dual)
			}
			if s.Inverted != tc.inverted {
				t.Errorf("inverted %v, want %v", s.Inverted, tc.inverted)
			}
			if s.Frames != int64(len(tc.a)) {
				t.Errorf("counted %d frames, want %d", s.Frames, len(tc.a))
			}
		})
	}
}

// TestPairIgnoresDCOffset is why the correlation is centered: an offset on one
// channel is a fault the per-channel statistics already report, and it must not
// change what the channels are said to have in common.
func TestPairIgnoresDCOffset(t *testing.T) {
	const n = 2048
	a := make([]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = 0.4 * math.Cos(2*math.Pi*float64((5*i)%n)/n)
		b[i] = a[i] + 0.3 // the same signal, sitting off zero
	}
	s := pairOf(t, a, b)
	if math.Abs(s.Correlation-1) > 1e-9 {
		t.Errorf("correlation %v, want 1 despite the offset", s.Correlation)
	}
	// The balance is not centered, and should not be: an offset really is
	// energy, and the level figures elsewhere count it too.
	if s.BalanceDB >= 0 {
		t.Errorf("balance %v dB, want the offset channel to read louder", s.BalanceDB)
	}
}

// TestPairStreamingMatchesWholeSignal checks that the sums do not depend on how
// the signal was handed over, which is what makes this usable from a block
// reader at all.
func TestPairStreamingMatchesWholeSignal(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 21))
	const n = 10000
	a := make([]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = r.NormFloat64()
		b[i] = 0.7*a[i] + 0.3*r.NormFloat64()
	}
	want := pairOf(t, a, b)

	for _, block := range []int{1, 3, 999, 10000} {
		p := NewPairBuilder(0, 1)
		for i := 0; i < n; i += block {
			hi := min(i+block, n)
			if err := p.Write(a[i:hi], b[i:hi]); err != nil {
				t.Fatal(err)
			}
		}
		got := closedPair(t, p)
		if got.Correlation != want.Correlation || got.BalanceDB != want.BalanceDB {
			t.Errorf("block %d: (%v, %v), want (%v, %v)",
				block, got.Correlation, got.BalanceDB, want.Correlation, want.BalanceDB)
		}
	}
}

// TestPairShorterBlockWins follows dsp's convention that a function given two
// slices uses the shorter of them.
func TestPairShorterBlockWins(t *testing.T) {
	p := NewPairBuilder(0, 1)
	if err := p.Write([]float64{1, 2, 3, 4, 5}, []float64{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if got := closedPair(t, p).Frames; got != 3 {
		t.Errorf("counted %d frames, want 3", got)
	}
}

func TestPairReset(t *testing.T) {
	p := NewPairBuilder(0, 1)
	if err := p.Write([]float64{1, 2}, []float64{1, 2}); err != nil {
		t.Fatal(err)
	}
	p.Reset()
	if _, err := p.Close(); err == nil {
		t.Error("Close after Reset returned a comparison; Reset left the frames behind")
	}
}

// TestPairNotesNameTheFault is what the numbers are for: a duplicate pair and an
// inverted one are both invisible in a per-channel report.
func TestPairNotesNameTheFault(t *testing.T) {
	const n = 1024
	a := make([]float64, n)
	inv := make([]float64, n)
	quiet := make([]float64, n)
	for i := range a {
		a[i] = 0.5 * math.Cos(2*math.Pi*float64((3*i)%n)/n)
		inv[i] = -a[i]
		quiet[i] = a[i] / 8 // 18 dB down
	}
	for _, tc := range []struct {
		name string
		a, b []float64
		want string
	}{
		{"dual mono", a, a, "mono written twice"},
		{"inverted", a, inv, "cancel to silence"},
		{"unbalanced", a, quiet, "above channel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := pairOf(t, tc.a, tc.b)
			if !strings.Contains(strings.Join(s.Notes, " "), tc.want) {
				t.Errorf("notes %q do not mention %q", s.Notes, tc.want)
			}
		})
	}
}
