package dsp

import (
	"math"
	"math/big"
	"math/cmplx"
	"math/rand/v2"
	"slices"
	"testing"
)

// TestOscillatorBinExact generates whole-cycle tones and checks each lands in
// its own bin and nowhere else.
func TestOscillatorBinExact(t *testing.T) {
	const n = 64
	for _, k := range []int{0, 1, 3, 16, 31, 32, -5} {
		o := must(NewOscillator[complex128](float64(k) / n))
		x := make([]complex128, n)
		o.Generate(x)
		for bin := range n {
			want := 0.0
			if bin == (k%n+n)%n {
				want = n
			}
			if got := DFTBin(x, bin); cmplx.Abs(got-complex(want, 0)) > 1e-12 {
				t.Errorf("k %d: bin %d = %v, want %v", k, bin, got, want)
			}
		}

		// A real tone splits between k and -k, which DFTBinReal folds back to
		// unit amplitude.
		if k == 0 || k == 32 {
			continue
		}
		r := must(NewOscillator[float64](float64(k) / n))
		y := make([]float64, n)
		r.Generate(y)
		for bin := 1; bin < n/2; bin++ {
			want := 0.0
			if bin == k || bin == -k {
				want = 1
			}
			if got := DFTBinReal(y, bin); cmplx.Abs(got-complex(want, 0)) > 1e-12 {
				t.Errorf("real k %d: bin %d = %v, want %v", k, bin, got, want)
			}
		}
	}
}

// TestOscillatorEveryWidth checks that each width is the complex128 tone
// narrowed: the real part for a real T, both parts for complex64.
func TestOscillatorEveryWidth(t *testing.T) {
	const f, n = 0.0731, 500
	ref := make([]complex128, n)
	must(NewOscillator[complex128](f)).Generate(ref)

	c64 := make([]complex64, n)
	must(NewOscillator[complex64](f)).Generate(c64)
	f64 := make([]float64, n)
	must(NewOscillator[float64](f)).Generate(f64)
	f32 := make([]float32, n)
	must(NewOscillator[float32](f)).Generate(f32)
	for i, z := range ref {
		if c64[i] != complex64(z) || f64[i] != real(z) || f32[i] != float32(real(z)) {
			t.Fatalf("sample %d: %v %v %v, want the narrowing of %v", i, c64[i], f64[i], f32[i], z)
		}
	}
}

// TestOscillatorSplits generates one block and then the same samples split at
// every point, which must be bit for bit the same.
func TestOscillatorSplits(t *testing.T) {
	const n = 200
	whole := make([]complex64, n)
	must(NewOscillator[complex64](-0.137)).Generate(whole)
	for split := range n + 1 {
		o := must(NewOscillator[complex64](-0.137))
		got := make([]complex64, n)
		o.Generate(got[:split])
		o.Generate(got[split:])
		if !slices.Equal(got, whole) {
			t.Fatalf("split at %d differs", split)
		}
	}
}

// TestOscillatorSetFrequency changes the frequency mid-stream and checks the
// tone continues from the phase it reached rather than restarting.
func TestOscillatorSetFrequency(t *testing.T) {
	const f1, f2, n1, n2 = 0.1, -0.23, 37, 50
	o := must(NewOscillator[complex128](f1))
	x := make([]complex128, n1+n2)
	o.Generate(x[:n1])
	if err := o.SetFrequency(f2); err != nil {
		t.Fatal(err)
	}
	o.Generate(x[n1:])
	if o.Frequency() != f2 {
		t.Errorf("Frequency = %v, want %v", o.Frequency(), f2)
	}
	for i, got := range x {
		phase := f1 * float64(i)
		if i >= n1 {
			phase = f1*n1 + f2*float64(i-n1)
		}
		if want := cmplx.Exp(complex(0, 2*math.Pi*phase)); cmplx.Abs(got-want) > 1e-12 {
			t.Fatalf("sample %d = %v, want %v", i, got, want)
		}
	}

	o.Reset()
	if o.Frequency() != f1 {
		t.Errorf("after Reset, Frequency = %v, want the constructor's %v", o.Frequency(), f1)
	}
	again := make([]complex128, n1)
	o.Generate(again)
	if !slices.Equal(again, x[:n1]) {
		t.Error("Reset did not restart the tone")
	}

	if err := o.SetFrequency(math.NaN()); err == nil {
		t.Error("SetFrequency accepted NaN")
	}
	for _, f := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if _, err := NewOscillator[float32](f); err == nil {
			t.Errorf("NewOscillator accepted %v", f)
		}
		if _, err := NewMixer[float32, complex64](f); err == nil {
			t.Errorf("NewMixer accepted %v", f)
		}
	}
}

// TestOscillatorPhaseExact checks that a binary-fraction frequency steps
// without error, so a quarter-rate tone is periodic to the bit.
func TestOscillatorPhaseExact(t *testing.T) {
	o := must(NewOscillator[complex128](0.25))
	x := make([]complex128, 4000)
	o.Generate(x)
	for i := 4; i < len(x); i++ {
		if x[i] != x[i%4] {
			t.Fatalf("sample %d = %v, sample %d = %v", i, x[i], i%4, x[i%4])
		}
	}
	if o.p.phase != 0 {
		t.Errorf("phase after 4000 quarter cycles = %v, want 0", o.p.phase)
	}
}

// TestOscillatorPhaseDrift runs 10^8 samples at a frequency that is not a
// binary fraction and compares the phase with the exact one: each step rounds
// by at most 2^-53 cycles, so the error is at most n*2^-53, 1.1e-8 cycles.
func TestOscillatorPhaseDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("10^8 samples")
	}
	const n = 100_000_000
	freq := 0.1234567 // 2^-53 is far coarser than its binary expansion ends
	o := must(NewOscillator[float32](freq))
	buf := make([]float32, 1<<16)
	for left := n; left > 0; {
		k := min(left, len(buf))
		o.Generate(buf[:k])
		left -= k
	}

	exact := new(big.Rat).Mul(new(big.Rat).SetFloat64(o.p.step), new(big.Rat).SetInt64(n))
	whole := new(big.Int).Quo(exact.Num(), exact.Denom())
	exact.Sub(exact, new(big.Rat).SetInt(whole))
	want, _ := exact.Float64()

	diff := math.Abs(o.p.phase - want)
	diff = min(diff, 1-diff) // either side of the wrap
	bound := n * math.Ldexp(1, -53)
	if diff > bound {
		t.Errorf("phase %v after %d samples, exact %v: error %.3g cycles exceeds %.3g", o.p.phase, n, want, diff, bound)
	}
	t.Logf("phase error after %d samples: %.3g cycles (bound %.3g)", n, diff, bound)
}

// TestMixerShift moves a tone by a whole number of bins up and down, and takes
// a real tone to its two images.
func TestMixerShift(t *testing.T) {
	const n = 64
	tone := make([]complex128, n)
	must(NewOscillator[complex128](5.0 / n)).Generate(tone)
	for _, shift := range []int{3, -3, -5, 20} {
		out := make([]complex128, n)
		must(NewMixer[complex128, complex128](float64(shift)/n)).Process(out, tone)
		want := ((5+shift)%n + n) % n
		for bin := range n {
			w := 0.0
			if bin == want {
				w = n
			}
			if got := DFTBin(out, bin); cmplx.Abs(got-complex(w, 0)) > 1e-11 {
				t.Errorf("shift %d: bin %d = %v, want %v", shift, bin, got, w)
			}
		}
	}

	// cos at bin 5 is half at +5 and half at -5; a shift of 3 puts them at 8
	// and -2.
	cosine := make([]float64, n)
	must(NewOscillator[float64](5.0 / n)).Generate(cosine)
	out := make([]complex64, n)
	must(NewMixer[float64, complex64](3.0/n)).Process(out, cosine)
	for bin := range n {
		w := 0.0
		if bin == 8 || bin == n-2 {
			w = n / 2
		}
		if got := complex128(DFTBin(out, bin)); cmplx.Abs(got-complex(w, 0)) > 1e-4 {
			t.Errorf("real input: bin %d = %v, want %v", bin, got, w)
		}
	}
}

// TestMixerMatchesRotator90 checks a quarter-rate Mixer against Rotator90,
// which does the same shift with no arithmetic. The Mixer's oscillator is a
// rounded cos and sin of multiples of pi/2, so they agree to within a few ulps
// of the sample's magnitude.
func TestMixerMatchesRotator90(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 5))
	x := make([]complex128, 1001)
	for i := range x {
		x[i] = complex(rng.NormFloat64(), rng.NormFloat64())
	}
	want := make([]complex128, len(x))
	NewRotator90[complex128]().Process(want, x)
	got := make([]complex128, len(x))
	m := must(NewMixer[complex128, complex128](0.25))
	m.Process(got[:500], x[:500])
	m.Process(got[500:], x[500:])
	for i := range x {
		if d := cmplx.Abs(got[i] - want[i]); d > 1e-15*cmplx.Abs(x[i])+1e-300 {
			t.Fatalf("sample %d: mixer %v, rotator %v", i, got[i], want[i])
		}
	}

	x64 := make([]complex64, len(x))
	for i, v := range x {
		x64[i] = complex64(v)
	}
	want64 := make([]complex64, len(x))
	NewRotator90[complex64]().Process(want64, x64)
	m64 := must(NewMixer[complex64, complex64](0.25))
	m64.Process(x64, x64) // in place
	for i := range x64 {
		if x64[i] != want64[i] {
			t.Fatalf("complex64 sample %d: mixer %v, rotator %v", i, x64[i], want64[i])
		}
	}
}

// BenchmarkOscillator and BenchmarkMixer report samples rather than bytes to
// SetBytes, so MB/s is millions of samples per second. They are the baseline
// for replacing the per-sample Sincos with a recurrence or a table.
func BenchmarkOscillator(b *testing.B) {
	b.Run("float32", func(b *testing.B) { benchOscillator[float32](b) })
	b.Run("complex64", func(b *testing.B) { benchOscillator[complex64](b) })
	b.Run("complex128", func(b *testing.B) { benchOscillator[complex128](b) })
}

func benchOscillator[T Sample](b *testing.B) {
	o := must(NewOscillator[T](0.0123))
	dst := make([]T, 4096)
	b.SetBytes(int64(len(dst)))
	for b.Loop() {
		o.Generate(dst)
	}
}

func BenchmarkMixer(b *testing.B) {
	b.Run("float32/complex64", func(b *testing.B) { benchMixer[float32, complex64](b) })
	b.Run("complex64/complex64", func(b *testing.B) { benchMixer[complex64, complex64](b) })
}

func benchMixer[In Sample, Out Complex](b *testing.B) {
	m := must(NewMixer[In, Out](0.0123))
	src := make([]In, 4096)
	dst := make([]Out, len(src))
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		m.Process(dst, src)
	}
}
