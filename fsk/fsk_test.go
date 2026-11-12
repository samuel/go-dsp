package fsk

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"samuelks.com/dsp"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

var (
	bell202 = Config{Mark: 1200, Space: 2200, Baud: 1200}
	bell103 = Config{Mark: 1270, Space: 1070, Baud: 300}
	// Bell 202 at complex baseband, tuned to its center frequency.
	bell202IQ = Config{Mark: -500, Space: 500, Baud: 1200}
)

func at(c Config, rate float64) Config {
	c.Rate = rate
	return c
}

// randomLevels returns n levels: an alternating preamble the clock locks on,
// then random data.
func randomLevels(seed uint64, preamble, n int) []byte {
	rng := rand.New(rand.NewPCG(seed, 1))
	l := make([]byte, n)
	for i := range l {
		if i < preamble {
			l[i] = byte(i & 1)
		} else {
			l[i] = byte(rng.IntN(2))
		}
	}
	return l
}

// modulate runs levels through a Modulator, with tail levels after them so the
// Demodulator's delay does not cut off the last symbol.
func modulate[T dsp.Sample](c Config, levels []byte) []T {
	m := must(NewModulator[T](c))
	src := append(slices.Clone(levels), 1, 1)
	x := make([]T, m.OutputLen(len(src)))
	m.Process(x, src)
	return x
}

func checkLevels(t *testing.T, what string, sent, got []byte) {
	t.Helper()
	if len(got) < len(sent) {
		t.Fatalf("%s: %d levels, want at least %d", what, len(got), len(sent))
	}
	for i := range sent {
		if got[i] != sent[i] {
			t.Fatalf("%s: level %d = %d, want %d", what, i, got[i], sent[i])
		}
	}
}

// TestRoundTripClean checks that a Modulator into a Demodulator returns every
// level, at whole and fractional samples per symbol, real and at complex
// baseband.
func TestRoundTripClean(t *testing.T) {
	levels := randomLevels(1, 0, 3000)
	for _, rate := range []float64{8000, 11025, 44100, 48000} {
		c := at(bell202, rate)
		checkLevels(t, fmt.Sprint("real ", rate),
			levels, must(NewDemodulator[float32](c)).Demodulate(nil, modulate[float32](c, levels)))
		ci := at(bell202IQ, rate)
		checkLevels(t, fmt.Sprint("complex ", rate),
			levels, must(NewDemodulator[complex64](ci)).Demodulate(nil, modulate[complex64](ci, levels)))
	}
	c := at(bell103, 8000)
	checkLevels(t, "bell103", levels, must(NewDemodulator[float64](c)).Demodulate(nil, modulate[float64](c, levels)))
}

// TestClockOffset transmits 1% fast and 1% slow. Random levels change often
// enough for the clock to follow.
func TestClockOffset(t *testing.T) {
	levels := randomLevels(2, 16, 5000)
	for _, rate := range []float64{11025, 48000} {
		for _, ppm := range []float64{-0.01, 0.01} {
			tx := at(bell202, rate)
			tx.Baud *= 1 + ppm
			got := must(NewDemodulator[float32](at(bell202, rate))).Demodulate(nil, modulate[float32](tx, levels))
			checkLevels(t, fmt.Sprintf("rate %v, clock %+.0f%%", rate, ppm*100), levels, got)
		}
	}
}

// TestLongRuns holds the level for up to 40 symbols at a time: with no
// transitions the clock free-runs, and must come out of each run on time.
func TestLongRuns(t *testing.T) {
	var levels []byte
	for run := 1; run <= 40; run++ {
		for range run {
			levels = append(levels, byte(run&1))
		}
	}
	for _, rate := range []float64{8000, 44100} {
		c := at(bell202, rate)
		checkLevels(t, fmt.Sprint(rate), levels, must(NewDemodulator[float32](c)).Demodulate(nil, modulate[float32](c, levels)))
	}
}

// TestNoise checks the level error rate with white noise added, against the
// rate an ideal noncoherent detector of orthogonal tones reaches,
// 0.5*exp(-Eb/2N0). Errors are counted in blocks re-aligned to the best
// offset, so a clock slip costs its block and is counted as well.
func TestNoise(t *testing.T) {
	const n = 50000
	levels := randomLevels(3, 32, n)
	for _, tc := range []struct {
		ebn0dB float64
		bound  float64 // as a multiple of the ideal error rate
		slips  int
	}{
		{10, 1.5, 2},
		{12, 2, 0},
	} {
		ebn0 := math.Pow(10, tc.ebn0dB/10)
		ideal := 0.5 * math.Exp(-ebn0/2)
		rng := rand.New(rand.NewPCG(4, uint64(tc.ebn0dB)))
		for _, rate := range []float64{11025, 48000} {
			// A real tone of unit amplitude has Eb = sps/2, and real noise of
			// variance N0/2 per sample is N0 per sample over both sidebands.
			c := at(bell202, rate)
			sps := c.Rate / c.Baud
			x := modulate[float64](c, levels)
			sigma := math.Sqrt(sps / 2 / ebn0 / 2)
			for i := range x {
				x[i] += rng.NormFloat64() * sigma
			}
			checkErrorRate(t, fmt.Sprintf("real %v Hz at %v dB", rate, tc.ebn0dB), levels,
				must(NewDemodulator[float64](c)).Demodulate(nil, x), ideal*tc.bound, tc.slips)

			// A unit complex tone has Eb = sps, and each part of the noise
			// carries half of N0.
			ci := at(bell202IQ, rate)
			z := modulate[complex128](ci, levels)
			sigma = math.Sqrt(sps / ebn0 / 2)
			for i := range z {
				z[i] += complex(rng.NormFloat64()*sigma, rng.NormFloat64()*sigma)
			}
			checkErrorRate(t, fmt.Sprintf("complex %v Hz at %v dB", rate, tc.ebn0dB), levels,
				must(NewDemodulator[complex128](ci)).Demodulate(nil, z), ideal*tc.bound, tc.slips)
		}
	}
}

func checkErrorRate(t *testing.T, what string, sent, got []byte, bound float64, maxSlips int) {
	t.Helper()
	errs, slips := alignedErrors(sent, got)
	rate := float64(errs) / float64(len(sent))
	if rate > bound || slips > maxSlips {
		t.Errorf("%s: error rate %.2e (bound %.2e), %d slips (bound %d)", what, rate, bound, slips, maxSlips)
	}
	t.Logf("%s: error rate %.2e, %d slips", what, rate, slips)
}

// alignedErrors counts level errors block by block, re-aligning each block of
// 64 to the offset that fits it best, so a clock slip costs its block rather
// than every level after it.
func alignedErrors(sent, got []byte) (errs, slips int) {
	off := 0
	for start := 0; start+64 <= len(sent); start += 64 {
		best, bestOff := 1<<30, off
		for d := -3; d <= 3; d++ {
			e := 0
			for i := start; i < start+64; i++ {
				j := i + off + d
				if j < 0 || j >= len(got) || got[j] != sent[i] {
					e++
				}
			}
			if e < best {
				best, bestOff = e, off+d
			}
		}
		if bestOff != off {
			slips++
		}
		off = bestOff
		errs += best
	}
	return errs, slips
}

// TestDemodulatorSplits feeds the same samples in blocks of every pattern and
// checks the levels are the same.
func TestDemodulatorSplits(t *testing.T) {
	c := at(bell202, 11025)
	x := modulate[float32](c, randomLevels(5, 8, 300))
	rng := rand.New(rand.NewPCG(6, 6))
	x2 := slices.Clone(x)
	for i := range x2 {
		x2[i] += float32(rng.NormFloat64() * 0.3)
	}
	for _, in := range [][]float32{x, x2} {
		want := must(NewDemodulator[float32](c)).Demodulate(nil, in)
		for _, size := range []int{1, 2, 3, 7, 255, 256, 257, 1000} {
			d := must(NewDemodulator[float32](c))
			var got []byte
			for i := 0; i < len(in); i += size {
				got = d.Demodulate(got, in[i:min(i+size, len(in))])
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("blocks of %d differ", size)
			}
		}
		d := must(NewDemodulator[float32](c))
		d.Demodulate(nil, in[:500])
		d.Reset()
		if got := d.Demodulate(nil, in); !bytes.Equal(got, want) {
			t.Fatal("Reset did not restart the demodulator")
		}
	}
}

// TestProcessors checks the Processor contract on the Discriminator and the
// Modulator: the count, block independence, Reset and a short dst.
func TestProcessors(t *testing.T) {
	c := at(bell202, 8000)
	x := modulate[float64](c, randomLevels(7, 0, 50))
	checkProcessor(t, func() dsp.Processor[float64, float32] {
		return must(NewDiscriminator[float64, float32](c))
	}, x)
	checkProcessor(t, func() dsp.Processor[complex64, float64] {
		return must(NewDiscriminator[complex64, float64](at(bell202IQ, 8000)))
	}, modulate[complex64](at(bell202IQ, 8000), randomLevels(8, 0, 50)))
	checkProcessor(t, func() dsp.Processor[byte, float32] {
		return must(NewModulator[float32](c))
	}, randomLevels(9, 0, 61))
	checkProcessor(t, func() dsp.Processor[byte, complex128] {
		return must(NewModulator[complex128](at(bell202IQ, 11025)))
	}, randomLevels(10, 0, 61))
}

func checkProcessor[In, Out any](t *testing.T, build func() dsp.Processor[In, Out], src []In) {
	t.Helper()
	p := build()
	need := p.OutputLen(len(src))
	want := make([]Out, need)
	if n := p.Process(want, src); n != need {
		t.Fatalf("Process wrote %d, OutputLen said %d", n, need)
	}
	for _, size := range []int{1, 2, 3, 7, 16} {
		p := build()
		var got []Out
		for i := 0; i < len(src); i += size {
			in := src[i:min(i+size, len(src))]
			out := make([]Out, p.OutputLen(len(in)))
			if n := p.Process(out, in); n != len(out) {
				t.Fatalf("blocks of %d: wrote %d, OutputLen said %d", size, n, len(out))
			}
			got = append(got, out...)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("blocks of %d differ", size)
		}
	}
	p.Reset()
	again := make([]Out, p.OutputLen(len(src)))
	p.Process(again, src)
	if fmt.Sprint(again) != fmt.Sprint(want) {
		t.Fatal("Reset did not restart")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("a short dst did not panic")
			}
		}()
		build().Process(make([]Out, need-1), src)
	}()
}

// TestModulatorSymbolLength checks that symbols at a fractional rate take
// floor(k*Rate/Baud) samples to end, so the stream never drifts from the
// symbol clock.
func TestModulatorSymbolLength(t *testing.T) {
	c := at(bell202, 44100) // 36.75 samples a symbol
	m := must(NewModulator[float32](c))
	total := 0
	for k := 1; k <= 1000; k++ {
		n := m.OutputLen(1)
		total += m.Process(make([]float32, n), []byte{byte(k & 1)})
		if want := int(math.Floor(float64(k) * 44100 / 1200)); total != want {
			t.Fatalf("after %d symbols, %d samples, want %d", k, total, want)
		}
	}
}

// TestModulatorPhaseContinuous checks that a level change does not jump the
// phase: no sample-to-sample step is larger than the higher tone's.
func TestModulatorPhaseContinuous(t *testing.T) {
	c := at(bell202, 48000)
	x := modulate[complex128](c, randomLevels(11, 0, 500))
	maxStep := 2 * math.Pi * c.Space / c.Rate
	for i := 1; i < len(x); i++ {
		step := math.Abs(cmplxAngle(x[i] / x[i-1]))
		if step > maxStep*(1+1e-9) {
			t.Fatalf("sample %d turns %.4f rad, more than the %.4f the space tone does", i, step, maxStep)
		}
	}
}

func cmplxAngle(z complex128) float64 { return math.Atan2(imag(z), real(z)) }

func TestConfigRejects(t *testing.T) {
	ok := at(bell202, 48000)
	for name, mutate := range map[string]func(*Config){
		"no rate":            func(c *Config) { c.Rate = 0 },
		"no baud":            func(c *Config) { c.Baud = 0 },
		"under 2 samples":    func(c *Config) { c.Baud = 30000 },
		"same tones":         func(c *Config) { c.Space = c.Mark },
		"past Nyquist":       func(c *Config) { c.Space = 24000 },
		"negative past":      func(c *Config) { c.Mark = -30000 },
		"NaN mark":           func(c *Config) { c.Mark = math.NaN() },
		"infinite rate":      func(c *Config) { c.Rate = math.Inf(1) },
		"loop gain 1":        func(c *Config) { c.LoopGain = 1 },
		"negative loop gain": func(c *Config) { c.LoopGain = -0.1 },
	} {
		c := ok
		mutate(&c)
		if _, err := NewModulator[float32](c); err == nil {
			t.Errorf("%s: NewModulator accepted it", name)
		}
		if _, err := NewSlicer[float32](c); err == nil {
			t.Errorf("%s: NewSlicer accepted it", name)
		}
		if _, err := NewDiscriminator[float32, float32](c); err == nil {
			t.Errorf("%s: NewDiscriminator accepted it", name)
		}
		if _, err := NewDemodulator[float32](c); err == nil {
			t.Errorf("%s: NewDemodulator accepted it", name)
		}
	}
	// Real samples cannot tell f from -f; complex ones can.
	mirror := Config{Mark: 500, Space: -500, Baud: 1200, Rate: 48000}
	if _, err := NewDiscriminator[float32, float32](mirror); err == nil {
		t.Error("a real discriminator accepted mirrored tones")
	}
	if _, err := NewDiscriminator[complex64, float32](mirror); err != nil {
		t.Errorf("a complex discriminator refused mirrored tones: %v", err)
	}
}

// TestSlicerAlone drives a Slicer with an ideal soft-decision square wave,
// one symbol per level, and checks it takes each level once.
func TestSlicerAlone(t *testing.T) {
	c := at(bell202, 9600) // 8 samples a symbol
	levels := randomLevels(12, 4, 400)
	soft := make([]float64, 0, 8*len(levels))
	for _, l := range levels {
		v := -1.0
		if l == 1 {
			v = 1
		}
		for range 8 {
			soft = append(soft, v)
		}
	}
	got := must(NewSlicer[float64](c)).Slice(nil, soft)
	checkLevels(t, "slicer", levels, got)
}

// FuzzSlicer feeds arbitrary samples, NaN and infinities among them: the
// Slicer must not panic, must take no more levels than its fastest clock
// allows, and must give the same levels however the input is split.
func FuzzSlicer(f *testing.F) {
	f.Add([]byte{0, 0, 128, 63, 0, 0, 128, 191}, uint16(1), uint8(3))
	f.Add(bytes.Repeat([]byte{0, 0, 192, 127}, 9), uint16(5), uint8(0))
	f.Fuzz(func(t *testing.T, in []byte, split uint16, baud uint8) {
		c := Config{Mark: 1200, Space: 2200, Baud: 300 + 100*float64(baud%38), Rate: 8000} // up to 4000, two samples a symbol
		src := make([]float32, len(in)/4)
		for i := range src {
			src[i] = math.Float32frombits(uint32(in[4*i]) | uint32(in[4*i+1])<<8 | uint32(in[4*i+2])<<16 | uint32(in[4*i+3])<<24)
		}
		whole := must(NewSlicer[float32](c)).Slice(nil, src)
		// The clock runs at most 1+LoopGain fast: a correction moves it at
		// most LoopGain/2 of a symbol, and there are at most two a symbol.
		if most := int(float64(len(src))*c.Baud/c.Rate*(1+DefaultLoopGain)) + 2; len(whole) > most {
			t.Fatalf("%d levels from %d samples, more than %d", len(whole), len(src), most)
		}
		k := int(split) % (len(src) + 1)
		s := must(NewSlicer[float32](c))
		parts := s.Slice(s.Slice(nil, src[:k]), src[k:])
		if !bytes.Equal(parts, whole) {
			t.Fatalf("split at %d differs", k)
		}
	})
}

func BenchmarkDemodulator(b *testing.B) {
	b.Run("float32/48000", func(b *testing.B) { benchDemodulator[float32](b, at(bell202, 48000)) })
	b.Run("complex64/48000", func(b *testing.B) { benchDemodulator[complex64](b, at(bell202IQ, 48000)) })
}

// benchDemodulator reports samples rather than bytes to SetBytes, so MB/s is
// millions of samples a second.
func benchDemodulator[T dsp.Sample](b *testing.B, c Config) {
	x := modulate[T](c, randomLevels(13, 0, 1200))
	d := must(NewDemodulator[T](c))
	var levels []byte
	b.SetBytes(int64(len(x)))
	for b.Loop() {
		levels = d.Demodulate(levels[:0], x)
	}
}
