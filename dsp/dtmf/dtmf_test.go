package dtmf

import (
	"encoding/binary"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

// readWAV returns the samples of a 16-bit mono PCM WAV file, scaled to
// [-1, 1). The captures in testdata are all in the same fixed 44-byte-header
// form, so the header is skipped rather than parsed.
func readWAV(t *testing.T, path string) []float32 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pcm := raw[44:]
	samples := make([]float32, len(pcm)/2)
	for i := range samples {
		samples[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
	}
	return samples
}

// decode runs a sliding window over samples and returns the digits, requiring
// the same key on three consecutive blocks before accepting it.
func decode(t *testing.T, d *Decoder, samples []float32, blockSize int) string {
	t.Helper()
	hop := blockSize / 4
	block := make([]float32, blockSize)
	var out []rune
	last, count := NoKey, 0
	for off := 0; off+hop <= len(samples); off += hop {
		copy(block, block[hop:])
		copy(block[blockSize-hop:], samples[off:off+hop])
		k, power := d.Feed(block)
		if k != NoKey && power <= 0 {
			t.Fatalf("decoded key %d with power %v", k, power)
		}
		if k == last && k != NoKey {
			if count++; count == 3 {
				out = append(out, Key(k))
			}
		} else {
			last, count = k, 0
		}
	}
	return string(out)
}

// TestDecodeCaptures decodes the captures in testdata, whose filenames carry
// the keys they hold, the tone length and the sample rate. Before Feed grew its
// validity tests it returned key 0 -- the digit '1' -- for silence, so a run
// over any of these came back mostly ones.
//
// Three captures cover what recorded audio can cover that synthesis cannot, and
// no more: Feed decodes one block in isolation, so everything about how digits
// follow one another is this file's decode helper rather than the package, and
// all sixteen keys are already checked against clean tones by
// TestFeedAcceptsEveryDigit. What is left is real envelopes and gaps at both
// rates the block size is derived for. 50 ms is the shortest tone the standard
// allows and so the hardest case -- more of each block straddles a tone edge --
// which is why there is no long-tone capture.
func TestDecodeCaptures(t *testing.T) {
	for _, tc := range []struct {
		path       string
		sampleRate float64
		want       string
	}{
		// Adjacent repeated digits, which need the gap between them resolved.
		{"testdata/1223445-50ms-8000.wav", 8000, "1223445"},
		// The fourth column and the symbol keys. "ps" is pound then star.
		{"testdata/AB1919ps-50ms-8000.wav", 8000, "AB1919#*"},
		// The only rate other than 8 kHz, so the only capture that exercises a
		// block size and Goertzel coefficients derived for a different one.
		{"testdata/0123456789-50ms-44100.wav", 44100, "0123456789"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			blockSize := int(205 * tc.sampleRate / 8000)
			d, err := NewStandard(tc.sampleRate, blockSize)
			if err != nil {
				t.Fatal(err)
			}
			got := decode(t, d, readWAV(t, tc.path), blockSize)
			if got != tc.want {
				t.Errorf("decoded %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFeedDoesNotModifyInput is the regression test for the window having been
// applied over the caller's slice. A sliding buffer keeps most of its samples
// from one call to the next, so those got windowed again on every block --
// progressively attenuated and smeared, which is exactly what the shipped
// example does.
func TestFeedDoesNotModifyInput(t *testing.T) {
	const blockSize = 205
	samples := make([]float32, blockSize)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i) * 0.1))
	}
	before := make([]float32, blockSize)
	copy(before, samples)

	d, err := NewStandard(8000, blockSize)
	if err != nil {
		t.Fatal(err)
	}
	d.Feed(samples)
	for i := range samples {
		if samples[i] != before[i] {
			t.Fatalf("Feed modified the input at %d: %v, was %v", i, samples[i], before[i])
		}
	}
}

// TestFeedRejects covers the blocks that must not decode as a digit.
func TestFeedRejects(t *testing.T) {
	const (
		sampleRate = 8000
		blockSize  = 205
	)
	tone := func(freqs ...float64) []float32 {
		s := make([]float32, blockSize)
		for i := range s {
			for _, f := range freqs {
				s[i] += float32(math.Sin(2 * math.Pi * f * float64(i) / sampleRate))
			}
		}
		return s
	}

	for _, tc := range []struct {
		name    string
		samples []float32
	}{
		{"silence", make([]float32, blockSize)},
		// One tone from each group is a digit; a single tone is not.
		{"low tone only", tone(697)},
		{"high tone only", tone(1209)},
		// Both from the same group.
		{"two low tones", tone(697, 770)},
		// Off-grid tones, e.g. speech.
		{"off grid", tone(500, 1100)},
		{"more off-grid tones", tone(311, 2749, 3001)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewStandard(sampleRate, blockSize)
			if err != nil {
				t.Fatal(err)
			}
			if k, power := d.Feed(tc.samples); k != NoKey {
				t.Errorf("decoded key %d (%c) with power %v, want NoKey", k, Key(k), power)
			}
		})
	}
}

// TestFeedThresholdIgnoresTheWindow checks that the energy floor means the same
// thing whatever window the caller supplies.
//
// The floor is a fraction of what a windowed sinusoid at a bin center produces,
// and that is a property of the window: sum(w)^2/(2*sum(w^2)) is 0.50*N for a
// rectangular window, 0.36*N for a Hamming and 0.29*N for a Blackman. Scaling
// by the block length instead, as this used to, folded that ratio into the
// threshold, so choosing a window quietly chose a detection sensitivity too.
//
// The measurement is how much interference a clean digit survives: a tone well
// outside both groups raises the block's energy without touching either bin, so
// the amplitude at which the digit stops decoding is where the floor sits.
// Those amplitudes agree to 3% across windows now and spanned 1.41x before.
func TestFeedThresholdIgnoresTheWindow(t *testing.T) {
	const (
		sampleRate = 8000
		blockSize  = 205
		// A bin center well clear of both groups, so it neither leaks into them
		// nor beats the winner in either.
		interferer = 77 * sampleRate / blockSize
	)

	critical := func(t *testing.T, windowFunc func([]float32)) float64 {
		t.Helper()
		d, err := New(stdLowFreq[:], stdHighFreq[:], sampleRate, blockSize, windowFunc)
		if err != nil {
			t.Fatal(err)
		}
		samples := make([]float32, blockSize)
		for amp := 0.0; amp < 12; amp += 0.01 {
			for i := range samples {
				u := 2 * math.Pi * float64(i) / sampleRate
				samples[i] = float32(math.Sin(697*u) + math.Sin(1209*u) + amp*math.Sin(interferer*u))
			}
			if k, _ := d.Feed(samples); k != 0 {
				return amp
			}
		}
		t.Fatal("the digit decoded at every interference level; the floor is never reached")
		return 0
	}

	var amps []float64
	for _, w := range []struct {
		name string
		fn   func([]float32)
	}{
		{"hamming", dsp.HammingWindow[float32]},
		{"blackman", dsp.BlackmanWindow[float32]},
		{"rectangular", func(s []float32) {
			for i := range s {
				s[i] = 1
			}
		}},
	} {
		t.Run(w.name, func(t *testing.T) {
			amps = append(amps, critical(t, w.fn))
			t.Logf("gives up at interference amplitude %.2f", amps[len(amps)-1])
		})
	}
	lo, hi := slices.Min(amps), slices.Max(amps)
	if hi/lo > 1.05 {
		t.Errorf("the digit survives to amplitudes %v under the three windows, a %.2fx spread; the threshold still moves with the window", amps, hi/lo)
	}
}

// TestFeedAcceptsEveryDigit checks that a clean synthetic pair decodes to the
// right key, for all sixteen.
func TestFeedAcceptsEveryDigit(t *testing.T) {
	const (
		sampleRate = 8000
		blockSize  = 205
	)
	low, high := StandardFreqs()
	for row, lowFreq := range low {
		for col, highFreq := range high {
			samples := make([]float32, blockSize)
			for i := range samples {
				t := float64(i) / sampleRate
				samples[i] = float32(math.Sin(2*math.Pi*lowFreq*t) +
					0.7*math.Sin(2*math.Pi*highFreq*t))
			}
			d, err := NewStandard(sampleRate, blockSize)
			if err != nil {
				t.Fatal(err)
			}
			k, power := d.Feed(samples)
			want := row*len(high) + col
			if k != want {
				t.Errorf("%g Hz + %g Hz decoded as %d, want %d (%c)", lowFreq, highFreq, k, want, Key(want))
			}
			if power <= 0 {
				t.Errorf("%g Hz + %g Hz reported power %v", lowFreq, highFreq, power)
			}
		}
	}
}

// TestFeedRejectsExcessiveTwist checks the twist limits: a pair whose two tones
// are far apart in level is not a valid digit.
func TestFeedRejectsExcessiveTwist(t *testing.T) {
	const (
		sampleRate = 8000
		blockSize  = 205
	)
	pair := func(lowGain, highGain float64) []float32 {
		s := make([]float32, blockSize)
		for i := range s {
			t := float64(i) / sampleRate
			s[i] = float32(lowGain*math.Sin(2*math.Pi*697*t) + highGain*math.Sin(2*math.Pi*1209*t))
		}
		return s
	}
	for _, tc := range []struct {
		name             string
		low, high        float64
		wantDecodedAsOne bool
	}{
		{"balanced", 1, 1, true},
		{"high 6 dB hot", 1, 2, true},
		{"high 20 dB hot", 1, 10, false},
		{"low 20 dB hot", 10, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewStandard(sampleRate, blockSize)
			if err != nil {
				t.Fatal(err)
			}
			k, _ := d.Feed(pair(tc.low, tc.high))
			if got := k == 0; got != tc.wantDecodedAsOne {
				t.Errorf("key = %d, decoded = %v, want decoded = %v", k, got, tc.wantDecodedAsOne)
			}
		})
	}
}

func TestFeedWrongBlockSizePanics(t *testing.T) {
	d, err := NewStandard(8000, 205)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 204, 206} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Feed with %d samples did not panic", n)
				}
			}()
			d.Feed(make([]float32, n))
		}()
	}
}

// TestKey covers the keypad accessor, including the out-of-range answers.
func TestKey(t *testing.T) {
	for i, want := range []rune{'1', '2', '3', 'A', '4', '5', '6', 'B', '7', '8', '9', 'C', '*', '0', '#', 'D'} {
		if got := Key(i); got != want {
			t.Errorf("Key(%d) = %q, want %q", i, got, want)
		}
	}
	for _, i := range []int{NoKey, -2, 16, 100} {
		if got := Key(i); got != 0 {
			t.Errorf("Key(%d) = %q, want 0", i, got)
		}
	}
}

// TestStandardFreqsAreCopies checks that the tone tables cannot be corrupted
// through the slices StandardFreqs hands out.
func TestStandardFreqsAreCopies(t *testing.T) {
	low, high := StandardFreqs()
	low[0], high[0] = 1, 1
	low2, high2 := StandardFreqs()
	if low2[0] != 697 || high2[0] != 1209 {
		t.Errorf("tables were corrupted: low %v high %v", low2, high2)
	}
}

// TestNewCustomFrequencies covers a non-standard tone grid, including the
// default window and the empty-group rejection.
func TestNewCustomFrequencies(t *testing.T) {
	const (
		sampleRate = 8000
		blockSize  = 205
	)
	// A two-by-two grid, with the default (nil) window function.
	low := []float64{600, 900}
	high := []float64{1400, 1700}
	d, err := New(low, high, sampleRate, blockSize, nil)
	if err != nil {
		t.Fatal(err)
	}
	for row, l := range low {
		for col, h := range high {
			samples := make([]float32, blockSize)
			for i := range samples {
				t := float64(i) / sampleRate
				samples[i] = float32(math.Sin(2*math.Pi*l*t) + math.Sin(2*math.Pi*h*t))
			}
			if k, _ := d.Feed(samples); k != row*len(high)+col {
				t.Errorf("%g + %g decoded as %d, want %d", l, h, k, row*len(high)+col)
			}
		}
	}

	for _, tc := range []struct {
		name      string
		low, high []float64
	}{
		{"no low", nil, high},
		{"no high", low, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.low, tc.high, sampleRate, blockSize, nil)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestNewRejects covers the arguments New has to catch before it allocates the
// window: a non-positive block size panics in make, and a non-positive sample
// rate reaches the bin builder only after that.
func TestNewRejects(t *testing.T) {
	low, high := StandardFreqs()
	for _, tc := range []struct {
		name  string
		rate  float64
		block int
		win   func([]float32)
	}{
		{"zero block", 8000, 0, nil},
		{"negative block", 8000, -205, nil},
		{"zero rate", 0, 205, nil},
		{"negative rate", -8000, 205, nil},
		{"NaN rate", math.NaN(), 205, nil},
		// A window of all zeros divides the energy floor by zero, so the
		// threshold becomes NaN and every block decodes as whatever is
		// loudest.
		{"zero window", 8000, 205, func(w []float32) { clear(w) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(low, high, tc.rate, tc.block, tc.win); err == nil {
				t.Error("New: expected an error")
			}
			if _, err := NewStandard(tc.rate, tc.block); err == nil && tc.win == nil {
				t.Error("NewStandard: expected an error")
			}
		})
	}
}
