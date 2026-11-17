package dsp

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

// pskSignal returns n samples of random PSK symbols of the given order at sps samples per
// symbol, rectangular pulses, rotated by a carrier at freq cycles per sample and phase0, and
// the symbols. QPSK symbols are at odd multiples of 45°.
func pskSignal(rng *rand.Rand, order, sps, n int, freq, phase0 float64) ([]complex64, []complex128) {
	syms := make([]complex128, n/sps+1)
	for i := range syms {
		k := rng.IntN(order)
		if order == 2 {
			syms[i] = complex(float64(2*k-1), 0)
		} else {
			syms[i] = cmplx.Rect(1, math.Pi/4+math.Pi/2*float64(k))
		}
	}
	out := make([]complex64, n)
	for i := range out {
		out[i] = complex64(syms[i/sps] * cmplx.Rect(1, 2*math.Pi*freq*float64(i)+phase0))
	}
	return out, syms
}

func TestCostasLocks(t *testing.T) {
	for _, order := range []int{2, 4} {
		rng := rand.New(rand.NewPCG(1, uint64(order)))
		const sps, n, freq = 8, 40000, 0.002
		in, syms := pskSignal(rng, order, sps, n, freq, 1.1)
		c, err := NewCostas[complex64](order, 0.005, 0.01)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]complex64, n)
		// Uneven blocks: the state carries across calls.
		for off, k := 0, 0; off < n; k++ {
			m := min([]int{1000, 333, 4096}[k%3], n-off)
			c.Process(out[off:off+m], in[off:off+m])
			off += m
		}
		if f := c.Frequency(); math.Abs(f-freq) > 1e-5 {
			t.Errorf("order %d: frequency %v, want %v", order, f, freq)
		}
		// Past the lock, every symbol is within a few degrees of a constellation point, rotated
		// by one of the modulation's ambiguities, the same for the rest of the run.
		ambiguity := 2 * math.Pi / float64(order)
		var rot float64
		first := true
		for i := n / 2; i < n; i++ {
			if i%sps != sps/2 {
				continue
			}
			d := cmplx.Phase(complex128(out[i]) / syms[i/sps])
			if first {
				rot, first = math.Round(d/ambiguity)*ambiguity, false
			}
			if e := math.Remainder(d-rot, 2*math.Pi); math.Abs(e) > 0.1 {
				t.Fatalf("order %d: symbol %d is %.3f rad off its point", order, i/sps, e)
			}
		}
	}
}

func TestCostasValidates(t *testing.T) {
	for _, tt := range []struct {
		order     int
		bw, maxHz float64
	}{{3, 0.01, 0.1}, {2, 0, 0.1}, {2, 0.5, 0.1}, {4, 0.01, 0.6}, {4, 0.01, -0.1}} {
		if _, err := NewCostas[complex64](tt.order, tt.bw, tt.maxHz); err == nil {
			t.Errorf("NewCostas(%d, %v, %v) succeeded", tt.order, tt.bw, tt.maxHz)
		}
	}
}

// shaped returns the symbols at sps samples per symbol through a raised-cosine pulse, with
// the first symbol's center at offset samples and the symbol spacing sps*(1+drift).
func shaped(syms []float64, sps, offset, drift float64) []float64 {
	period := sps * (1 + drift)
	n := int(float64(len(syms))*period + offset)
	out := make([]float64, n)
	for k, a := range syms {
		center := offset + float64(k)*period
		lo, hi := max(0, int(center-8*period)), min(n, int(center+8*period)+1)
		for i := lo; i < hi; i++ {
			out[i] += a * raisedCosine((float64(i)-center)/period, 0.5)
		}
	}
	return out
}

func randomBits(rng *rand.Rand, n int) []float64 {
	syms := make([]float64, n)
	for i := range syms {
		syms[i] = float64(2*rng.IntN(2) - 1)
	}
	return syms
}

// matchSymbols finds where want starts in got, after a lock-up of skip outputs, and returns
// the number of symbols compared and how many differ in sign, allowing a slip of up to two
// symbols either way.
func matchSymbols(got []float64, want []float64, skip int) (int, int) {
	bestErr, bestN := math.MaxInt, 0
	for shift := -3; shift <= 3; shift++ {
		var n, errs int
		for i := skip; i < len(got); i++ {
			j := i + shift
			if j < 0 || j >= len(want) {
				continue
			}
			n++
			if (got[i] > 0) != (want[j] > 0) {
				errs++
			}
		}
		if n > 0 && errs < bestErr {
			bestErr, bestN = errs, n
		}
	}
	return bestN, bestErr
}

func TestSymbolSyncRecoversSymbols(t *testing.T) {
	for _, det := range []TimingDetector{Gardner, MuellerMuller} {
		for _, tt := range []struct {
			sps, offset, drift float64
		}{
			{8, 3.3, 0},
			{8, 0.5, 0.004},
			{5.3, 2.1, -0.004},
			{4, 1.7, 0.002},
		} {
			rng := rand.New(rand.NewPCG(2, uint64(det)))
			syms := randomBits(rng, 4000)
			in := shaped(syms, tt.sps, tt.offset, tt.drift)
			// A level far from 1 checks the normalization.
			for i := range in {
				in[i] *= 0.05
			}
			s, err := NewSymbolSync[float32](SymbolSyncConfig{
				SamplesPerSymbol: tt.sps, LoopBandwidth: 0.02, MaxDeviation: 0.01, Detector: det,
			})
			if err != nil {
				t.Fatal(err)
			}
			src := make([]float32, len(in))
			for i, v := range in {
				src[i] = float32(v)
			}
			var got []float64
			for off, k := 0, 0; off < len(src); k++ {
				m := min([]int{1000, 333, 4096, 7}[k%4], len(src)-off)
				dst := make([]float32, s.OutputLen(m))
				w := s.Process(dst, src[off:off+m])
				for _, v := range dst[:w] {
					got = append(got, float64(v))
				}
				off += m
			}
			n, errs := matchSymbols(got, syms, 500)
			if n < len(syms)-600 || errs > 0 {
				t.Errorf("detector %d, %+v: %d symbol errors in %d (%d out)", det, tt, errs, n, len(got))
			}
			// Past the lock, the symbols sit near the ideal ±0.05: the clock is centered.
			var spread float64
			for _, v := range got[len(got)/2:] {
				spread = max(spread, math.Abs(math.Abs(v)-0.05))
			}
			if spread > 0.012 {
				t.Errorf("detector %d, %+v: symbols as far as %.4f from ±0.05", det, tt, spread)
			}
			if want := tt.sps * (1 + tt.drift); math.Abs(s.SamplesPerSymbol()-want) > 0.002*want {
				t.Errorf("detector %d, %+v: tracked %.4f samples per symbol, want %.4f", det, tt, s.SamplesPerSymbol(), want)
			}
		}
	}
}

func TestSymbolSyncComplex(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	re, im := randomBits(rng, 3000), randomBits(rng, 3000)
	a, b := shaped(re, 6, 2.5, 0.002), shaped(im, 6, 2.5, 0.002)
	src := make([]complex64, len(a))
	for i := range src {
		src[i] = complex(float32(a[i]), float32(b[i]))
	}
	s, err := NewSymbolSync[complex64](SymbolSyncConfig{SamplesPerSymbol: 6, LoopBandwidth: 0.02, MaxDeviation: 0.01})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]complex64, s.OutputLen(len(src)))
	dst = dst[:s.Process(dst, src)]
	gotRe, gotIm := make([]float64, len(dst)), make([]float64, len(dst))
	for i, v := range dst {
		gotRe[i], gotIm[i] = float64(real(v)), float64(imag(v))
	}
	if n, errs := matchSymbols(gotRe, re, 300); errs > 0 || n < 2500 {
		t.Errorf("I: %d errors in %d", errs, n)
	}
	if n, errs := matchSymbols(gotIm, im, 300); errs > 0 || n < 2500 {
		t.Errorf("Q: %d errors in %d", errs, n)
	}
}

func TestSymbolSyncOutputLenBounds(t *testing.T) {
	s, err := NewSymbolSync[float64](SymbolSyncConfig{SamplesPerSymbol: 2, LoopBandwidth: 0.1, MaxDeviation: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(4, 4))
	for range 200 {
		src := make([]float64, rng.IntN(50))
		for i := range src {
			src[i] = rng.NormFloat64()
		}
		dst := make([]float64, s.OutputLen(len(src)))
		// Process panics if the bound is short, and writes no more than it.
		if n := s.Process(dst, src); n > len(dst) {
			t.Fatalf("wrote %d, bound %d", n, len(dst))
		}
	}
	for _, c := range []SymbolSyncConfig{
		{SamplesPerSymbol: 1.5, LoopBandwidth: 0.01},
		{SamplesPerSymbol: 4, LoopBandwidth: 0},
		{SamplesPerSymbol: 4, LoopBandwidth: 0.01, MaxDeviation: 0.5},
		{SamplesPerSymbol: 4, LoopBandwidth: 0.01, Detector: 7},
	} {
		if _, err := NewSymbolSync[float32](c); err == nil {
			t.Errorf("NewSymbolSync(%+v) succeeded", c)
		}
	}
}

func TestRaisedCosineFIRs(t *testing.T) {
	const sps, span, beta = 8, 32, 0.35
	n := span*sps + 1
	rrc, err := RootRaisedCosineFIR[float64](n, sps, beta)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := RaisedCosineFIR[float64](n, sps, beta)
	if err != nil {
		t.Fatal(err)
	}
	var energy float64
	for k := range rrc {
		energy += rrc[k] * rrc[k]
		if rrc[k] != rrc[n-1-k] || rc[k] != rc[n-1-k] {
			t.Fatalf("tap %d is not symmetric", k)
		}
	}
	if math.Abs(energy-1) > 1e-12 {
		t.Errorf("RRC energy %v, want 1", energy)
	}
	// The raised cosine is 1 at its center and 0 at every other symbol.
	c := n / 2
	for k := -span / 2; k <= span/2; k++ {
		want := 0.0
		if k == 0 {
			want = 1
		}
		if v := rc[c+k*sps]; math.Abs(v-want) > 1e-12 {
			t.Errorf("RC at symbol %d = %v, want %v", k, v, want)
		}
	}
	// RRC through RRC is the raised cosine: no intersymbol interference at the symbol spacing,
	// up to what truncating the pulses leaves.
	conv := make([]float64, 2*n-1)
	for i, a := range rrc {
		for j, b := range rrc {
			conv[i+j] += a * b
		}
	}
	cc := n - 1
	for k := -span / 2; k <= span/2; k++ {
		if got, want := conv[cc+k*sps], rc[c+k*sps]; math.Abs(got-want) > 2e-3 {
			t.Errorf("RRC⊗RRC at symbol %d = %v, want %v", k, got, want)
		}
	}
	for off := -sps * 4; off <= sps*4; off++ {
		if got, want := conv[cc+off], rc[c+off]; math.Abs(got-want) > 5e-3 {
			t.Errorf("RRC⊗RRC at %d samples = %v, raised cosine %v", off, got, want)
		}
	}
	// Special points: the zeros of the formulas' denominators are finite.
	for _, beta := range []float64{0, 0.25, 0.5, 1} {
		taps, err := RootRaisedCosineFIR[float32](65, 4, beta)
		if err != nil {
			t.Fatal(err)
		}
		rc, err := RaisedCosineFIR[float32](65, 4, beta)
		if err != nil {
			t.Fatal(err)
		}
		for k := range taps {
			if math.IsNaN(float64(taps[k])) || math.IsInf(float64(taps[k]), 0) || math.IsNaN(float64(rc[k])) {
				t.Fatalf("rolloff %v: tap %d is %v, %v", beta, k, taps[k], rc[k])
			}
		}
	}
	for _, bad := range []struct {
		n            int
		sps, rolloff float64
	}{{0, 4, 0.5}, {11, 0.5, 0.5}, {11, 4, -0.1}, {11, 4, 1.5}, {11, math.NaN(), 0.5}} {
		if _, err := RootRaisedCosineFIR[float64](bad.n, bad.sps, bad.rolloff); err == nil {
			t.Errorf("RootRaisedCosineFIR(%+v) succeeded", bad)
		}
	}
}
