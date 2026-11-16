package dsp

import (
	"math"
	"testing"
)

func peak(s []float64) float64 {
	var p float64
	for _, v := range s {
		p = max(p, math.Abs(v))
	}
	return p
}

func TestAGCHoldsTheTarget(t *testing.T) {
	cfg := AGCConfig{Target: 0.5, Attack: 10, Decay: 2000, MaxGain: 1000}
	for _, amp := range []float64{0.01, 0.5, 4} {
		a, err := NewAGC[float64](cfg)
		if err != nil {
			t.Fatal(err)
		}
		in := sineF64(40000, 0.01)
		for i := range in {
			in[i] *= amp
		}
		out := make([]float64, len(in))
		a.Process(out, in)
		// The envelope tracks peaks, so the settled peak is close to the target.
		if p := peak(out[len(out)-2000:]); math.Abs(p-cfg.Target) > 0.05 {
			t.Errorf("amplitude %v: settled peak %v, want about %v", amp, p, cfg.Target)
		}
	}

	// Silence is raised no further than MaxGain.
	a, err := NewAGC[complex64](AGCConfig{Target: 1, Attack: 1, Decay: 10, MaxGain: 10})
	if err != nil {
		t.Fatal(err)
	}
	in := make([]complex64, 1000)
	for i := range in {
		in[i] = 0.001
	}
	a.Process(in, in)
	if g := a.Gain(); g != 10 {
		t.Errorf("gain on a quiet signal = %v, want the cap 10", g)
	}
	if v := in[len(in)-1]; math.Abs(float64(real(v))-0.01) > 1e-6 {
		t.Errorf("quiet output = %v, want 0.01", v)
	}

	for _, bad := range []AGCConfig{{}, {Target: 1, Attack: 1, Decay: 1, MaxGain: 0.5}, {Target: 1, Attack: 0, Decay: 1, MaxGain: 1}} {
		if _, err := NewAGC[float32](bad); err == nil {
			t.Errorf("config %+v accepted", bad)
		}
	}
}

func TestPowerSquelch(t *testing.T) {
	s, err := NewPowerSquelch[float64](PowerSquelchConfig{Threshold: -20, Hysteresis: 6, Average: 10})
	if err != nil {
		t.Fatal(err)
	}
	level := func(amp float64, n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = amp
		}
		s.Process(out, out)
		return out
	}
	// -30 dB stays closed and silent.
	if out := level(math.Pow(10, -30.0/20), 200); s.Open() || peak(out) != 0 {
		t.Errorf("-30 dB: open %v, peak %v", s.Open(), peak(out))
	}
	// -10 dB opens and passes.
	if out := level(math.Pow(10, -10.0/20), 200); !s.Open() || out[len(out)-1] == 0 {
		t.Error("-10 dB did not open the squelch")
	}
	// -23 dB is inside the hysteresis, so it stays open.
	if level(math.Pow(10, -23.0/20), 200); !s.Open() {
		t.Error("-23 dB closed the squelch inside the hysteresis")
	}
	// -30 dB closes it.
	if level(math.Pow(10, -30.0/20), 200); s.Open() {
		t.Error("-30 dB did not close the squelch")
	}
	if p := s.Power(); math.Abs(p+30) > 0.5 {
		t.Errorf("power = %v dB, want about -30", p)
	}
	if _, err := NewPowerSquelch[float32](PowerSquelchConfig{Threshold: 0, Hysteresis: -1, Average: 1}); err == nil {
		t.Error("negative hysteresis accepted")
	}
}
