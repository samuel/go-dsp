package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestGoertzel(t *testing.T) {
	samplerate := 1024.0
	blocksize := 1024
	freq := 128
	samples := make([]float64, blocksize)
	w := 2 * math.Pi / samplerate
	for i := range blocksize {
		samples[i] = math.Sin(float64(i) * float64(freq) * w)
	}
	g, err := NewGoertzel[float64]([]float64{128, 129}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(samples)
	m := g.Power()
	if e := math.Pow(float64(blocksize)/2, 2); !approxEqual(m[0], e, 1e-8) {
		t.Errorf("Goertzel magnitude = %f. Want %f", m[0], e)
	}
	if !approxEqual(m[1], 0.0, 1e-10) {
		t.Errorf("Foertzel magnitude = %f. Want 0.0", m[1])
	}
	c := g.Complex()
	if e, m := math.Sqrt(math.Pow(float64(blocksize)/2, 2)), cmplx.Abs(c[0]); !approxEqual(m, e, 1e-8) {
		t.Errorf("Goertzel magnitude = %f. Want %f", m, e)
	}
	if e, p := -math.Pi/2, cmplx.Phase(c[0]); !approxEqual(p, e, 1e-12) {
		t.Errorf("Goertzel phase = %f. Want %f", p, e)
	}
}

// TestGoertzelFloat32Samples checks the float32 instantiation against the same
// closed form as TestGoertzel, since the recursion runs in float64 either way.
func TestGoertzelFloat32Samples(t *testing.T) {
	const (
		samplerate = 1024
		blocksize  = 1024
		freq       = 128
	)
	samples := make([]float32, blocksize)
	w := 2 * math.Pi / samplerate
	for i := range blocksize {
		samples[i] = float32(math.Sin(float64(i) * freq * w))
	}
	g, err := NewGoertzel[float32]([]float64{freq, freq + 1}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(samples)
	m := g.Power()
	if e := float64(blocksize / 2 * (blocksize / 2)); !approxEqual(m[0], e, e*1e-4) {
		t.Errorf("Goertzel[float32] power = %f. Want %f", m[0], e)
	}
	if !approxEqual(m[1], 0, float64(blocksize/2*(blocksize/2))*1e-6) {
		t.Errorf("Goertzel[float32] off-bin power = %f. Want 0", m[1])
	}
	c := g.Complex()
	if e, got := blocksize/2.0, cmplx.Abs(c[0]); !approxEqual(got, e, e*1e-4) {
		t.Errorf("Goertzel[float32] magnitude = %f. Want %f", got, e)
	}
	if e, p := -math.Pi/2, cmplx.Phase(c[0]); !approxEqual(p, e, 1e-5) {
		t.Errorf("Goertzel[float32] phase = %f. Want %f", p, e)
	}

	// Reset has to leave the filter as new.
	g.Reset()
	g.Feed(samples)
	if m2 := g.Power(); m2[0] != m[0] {
		t.Errorf("after Reset power = %f, want %f", m2[0], m[0])
	}
}

// TestComplexGoertzelSignDiscrimination is the property ComplexGoertzel exists
// for: running the recursion over the real and imaginary parts separately tells
// a positive frequency from its negative image, which a real Goertzel cannot.
func TestComplexGoertzelSignDiscrimination(t *testing.T) {
	const (
		samplerate = 1024
		blocksize  = 1024
		freq       = 128
	)
	w := 2 * math.Pi * freq / samplerate

	positive := make([]complex128, blocksize)
	negative := make([]complex128, blocksize)
	real128 := make([]float64, blocksize)
	for i := range blocksize {
		positive[i] = cmplx.Exp(complex(0, w*float64(i)))
		negative[i] = cmplx.Exp(complex(0, -w*float64(i)))
		real128[i] = math.Cos(w * float64(i))
	}

	powerAt := func(samples []complex128) float64 {
		g, err := NewComplexGoertzel[complex128]([]float64{freq}, samplerate, blocksize)
		if err != nil {
			t.Fatal(err)
		}
		g.Feed(samples)
		return g.Power()[0]
	}
	pos, neg := powerAt(positive), powerAt(negative)
	if pos < 1e6*neg {
		t.Errorf("power at +f is %g and at -f is %g; expected the negative image to be rejected", pos, neg)
	}
	if e := float64(blocksize * blocksize); !approxEqual(pos, e, e*1e-8) {
		t.Errorf("power at +f = %g, want %g", pos, e)
	}

	// The real Goertzel sees a cosine as both images at half amplitude each, so
	// it cannot tell them apart -- the contrast the test above measures.
	g, err := NewGoertzel[float64]([]float64{freq}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(real128)
	if e := float64(blocksize / 2 * (blocksize / 2)); !approxEqual(g.Power()[0], e, e*1e-8) {
		t.Errorf("real Goertzel power = %g, want %g", g.Power()[0], e)
	}

	// Complex and Power have to agree.
	gc, err := NewComplexGoertzel[complex128]([]float64{freq}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	gc.Feed(positive)
	if got, want := cmplx.Abs(gc.Complex()[0]), math.Sqrt(gc.Power()[0]); !approxEqual(got, want, want*1e-9) {
		t.Errorf("|Complex()| = %g, sqrt(Power()) = %g", got, want)
	}

	gc.Reset()
	gc.Feed(positive)
	if got := gc.Power()[0]; !approxEqual(got, pos, pos*1e-9) {
		t.Errorf("after Reset power = %g, want %g", got, pos)
	}
}

// TestGoertzelFeedProtocol covers the block accounting. Feeding more than
// blockSize samples, or reading a result before a whole block is in, used to be
// silently wrong: the recursion had run for a different number of samples than
// the bin it was built for.
func TestGoertzelFeedProtocol(t *testing.T) {
	t.Run("overfeed", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		g, err := NewGoertzel[float64]([]float64{100}, 8000, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.Feed(make([]float64, 9))
	})
	t.Run("overfeed across calls", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		g, err := NewGoertzel[float64]([]float64{100}, 8000, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.Feed(make([]float64, 5))
		g.Feed(make([]float64, 5))
	})
	t.Run("underfeed", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		g, err := NewGoertzel[float64]([]float64{100}, 8000, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.Feed(make([]float64, 4))
		g.Power()
	})
	t.Run("complex underfeed", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		g, err := NewComplexGoertzel[complex128]([]float64{100}, 8000, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.Complex()
	})
	t.Run("a block in pieces", func(t *testing.T) {
		g, err := NewGoertzel[float64]([]float64{100}, 8000, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.Feed(make([]float64, 3))
		g.Feed(make([]float64, 5))
		g.Power()
		// Reset puts the count back, so the next block can be fed.
		g.Reset()
		g.Feed(make([]float64, 8))
		g.Complex()
	})
}

func TestNewGoertzelRejects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rate  float64
		block int
	}{
		{"zero rate", 0, 8},
		{"negative rate", -8000, 8},
		{"NaN rate", math.NaN(), 8},
		{"zero block", 8000, 0},
		{"negative block", 8000, -8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewGoertzel[float64]([]float64{100}, tc.rate, tc.block); err == nil {
				t.Error("NewGoertzel: expected an error")
			}
			// The two constructors share newGoertzelBins, so both have to
			// reach it before allocating.
			if _, err := NewComplexGoertzel[complex128]([]float64{100}, tc.rate, tc.block); err == nil {
				t.Error("NewComplexGoertzel: expected an error")
			}
		})
	}
}

// TestGoertzelFractionalAndNegativeFreq covers what the old []uint64 target
// list could not express: a frequency that is not a whole number of Hz, and a
// negative one, which is meaningful only for the complex detector.
func TestGoertzelFractionalAndNegativeFreq(t *testing.T) {
	const (
		samplerate = 1000
		blocksize  = 1000
	)
	// 128.4 Hz rounds to bin 128 at this rate and block size, so it detects the
	// same tone as 128 Hz; the point is that it is accepted at all.
	samples := make([]float64, blocksize)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 128 * float64(i) / samplerate)
	}
	g, err := NewGoertzel[float64]([]float64{128.4}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(samples)
	if e := float64(blocksize / 2 * (blocksize / 2)); !approxEqual(g.Power()[0], e, e*1e-8) {
		t.Errorf("power at 128.4 Hz = %g, want %g", g.Power()[0], e)
	}

	// A negative target frequency picks out the negative image, which is what a
	// complex Goertzel is for.
	neg := make([]complex128, blocksize)
	for i := range neg {
		neg[i] = cmplx.Exp(complex(0, -2*math.Pi*128*float64(i)/samplerate))
	}
	gc, err := NewComplexGoertzel[complex128]([]float64{-128}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	gc.Feed(neg)
	if e := float64(blocksize * blocksize); !approxEqual(gc.Power()[0], e, e*1e-8) {
		t.Errorf("power at -128 Hz = %g, want %g", gc.Power()[0], e)
	}
	gcPos, err := NewComplexGoertzel[complex128]([]float64{128}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	gcPos.Feed(neg)
	if got := gcPos.Power()[0]; got > 1e-10 {
		t.Errorf("+128 Hz detector saw %g on a -128 Hz tone, want ~0", got)
	}
}

// TestComplexGoertzel64 covers the complex64 instantiation.
func TestComplexGoertzel64(t *testing.T) {
	const (
		samplerate = 1024
		blocksize  = 1024
		freq       = 128
	)
	samples := make([]complex64, blocksize)
	for i := range samples {
		samples[i] = complex64(cmplx.Exp(complex(0, 2*math.Pi*freq*float64(i)/samplerate)))
	}
	g, err := NewComplexGoertzel[complex64]([]float64{freq}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(samples)
	if e := float64(blocksize * blocksize); !approxEqual(g.Power()[0], e, e*1e-4) {
		t.Errorf("power = %g, want %g", g.Power()[0], e)
	}
}

// TestGoertzelReset covers Reset on the float64 filter.
func TestGoertzelReset(t *testing.T) {
	const (
		samplerate = 1024
		blocksize  = 1024
		freq       = 128
	)
	samples := make([]float64, blocksize)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / samplerate)
	}
	g, err := NewGoertzel[float64]([]float64{freq}, samplerate, blocksize)
	if err != nil {
		t.Fatal(err)
	}
	g.Feed(samples)
	first := g.Power()[0]
	g.Reset()
	g.Feed(samples)
	if got := g.Power()[0]; got != first {
		t.Errorf("after Reset power = %v, want %v", got, first)
	}
}
