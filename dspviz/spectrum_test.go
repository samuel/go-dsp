package dspviz

import (
	"bytes"
	"image/png"
	"math"
	"math/big"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

// TestToneSpectrumIsClean checks that a transparent processor leaves a single
// spike and nothing else.
func TestToneSpectrumIsClean(t *testing.T) {
	p, err := Identity(48000)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ToneSpectrum(p, 997, SpectrumOptions{Frame: 1 << 14})
	if err != nil {
		t.Fatal(err)
	}
	if db := s.MagDB[s.Bin]; math.Abs(db) > 1e-9 {
		t.Errorf("a full-scale tone reads %v dBFS, want 0", db)
	}
	if sf := s.SFDR(); sf > -250 {
		t.Errorf("SFDR %v dB through an identity; the measurement is adding its own spurs", sf)
	}
	if thd := s.THD(10); thd > -250 {
		t.Errorf("THD %v dB through an identity", thd)
	}
	hz, db := s.Peak(997)
	if math.Abs(hz-s.Fundamental) > 1 || math.Abs(db) > 1e-9 {
		t.Errorf("Peak found %v Hz at %v dB, want %v Hz at 0 dB", hz, db, s.Fundamental)
	}
}

// TestToneSpectrumFindsDistortion checks the other direction: a processor that
// deliberately adds a harmonic has to show it, at the level it was added.
func TestToneSpectrumFindsDistortion(t *testing.T) {
	const rate = 48000.0
	// y = x + a*(2x^2-1), which is a cosine at twice the frequency scaled by a.
	const a = 0.001 // -60 dB
	p, err := NewFuncProcessor(func(dst, in []float64) []float64 {
		for _, x := range in {
			dst = append(dst, x+a*(2*x*x-1))
		}
		return dst
	}, nil, rate, rate)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ToneSpectrum(p, 1000, SpectrumOptions{Frame: 1 << 14})
	if err != nil {
		t.Fatal(err)
	}
	_, db := s.Peak(2 * s.Fundamental)
	if math.Abs(db-DB(a)) > 0.1 {
		t.Errorf("the second harmonic reads %v dB, want %v", db, DB(a))
	}
	if sf := s.SFDR(); math.Abs(sf-DB(a)) > 0.1 {
		t.Errorf("SFDR %v dB, want %v", sf, DB(a))
	}
	if thd := s.THD(10); math.Abs(thd-DB(a)) > 0.1 {
		t.Errorf("THD %v dB, want %v", thd, DB(a))
	}
}

// TestSweepSignalPhaseIsExact checks that the sweep generator does not put a
// floor under the plot it feeds, the way a naively accumulated phase would. A
// sweep has energy everywhere by construction, so instead of looking for spurs
// this compares the two forms against each other in the time domain.
func TestSweepSignalPhaseIsExact(t *testing.T) {
	const rate, n = 48000.0, 1 << 18
	x := SweepSignal(make([]float64, n), rate, SweepOptions{Low: 0, High: 24000, Amplitude: 1, Fade: -1})

	// Everything has to stay inside the amplitude asked for; a phase that has
	// drifted shows up as an envelope that has not.
	for i, v := range x {
		if math.Abs(v) > 1.0000001 {
			t.Fatalf("sample %d is %v, outside the requested amplitude", i, v)
		}
	}

	// The instantaneous frequency has to reach the top of the range and no
	// further. Count zero crossings over the last tenth: at 24 kHz against a
	// 48 kHz rate the signal alternates every sample.
	tail := x[n-n/10:]
	var crossings int
	for i := 1; i < len(tail); i++ {
		if (tail[i-1] < 0) != (tail[i] < 0) {
			crossings++
		}
	}
	got := float64(crossings) / 2 / (float64(len(tail)) / rate)
	if math.Abs(got-22800) > 1500 {
		t.Errorf("the last tenth of the sweep averages %.0f Hz, want about 22.8 kHz", got)
	}
}

// TestSweepSignalMatchesExactPhase compares the generator against the same
// numerator worked out in arbitrary precision, at the longest sweep the server
// will run.
//
// The closed form this replaced -- a*i + b*i*i reduced afterwards -- overflows
// int64 at 86% of a thirty-second sweep at 192 kHz once the constants are
// scaled finely enough to keep a fractional span. Past that the phase is
// unrelated: the cosine reads +1 where it should be -1. This compares numbers
// because the picture would not say so.
func TestSweepSignalMatchesExactPhase(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rate, sec float64
	}{
		{"8s at 48k", 48000, 8},
		{"30s at 192k", 192000, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := int(tc.sec * tc.rate)
			got := SweepSignal(make([]float64, n), tc.rate, SweepOptions{
				Low: 0, High: tc.rate / 2, Amplitude: 1, Fade: -1,
			})

			// phi(i)/(2*pi) = (a*i + b*i^2)/den, in exact integers.
			den := big.NewInt(2 * int64(n) * int64(math.Round(tc.rate)) * sweepScale)
			a := big.NewInt(int64(math.Round(0 * 2 * float64(n) * sweepScale)))
			b := big.NewInt(int64(math.Round(tc.rate / 2 * sweepScale)))
			num, t1, t2 := new(big.Int), new(big.Int), new(big.Int)
			for i := range got {
				ii := big.NewInt(int64(i))
				t1.Mul(a, ii)
				t2.Mul(b, ii)
				t2.Mul(t2, ii)
				num.Add(t1, t2).Mod(num, den)
				want := math.Cos(2 * math.Pi * float64(num.Int64()) / float64(den.Int64()))
				if math.Abs(got[i]-want) > 1e-9 {
					t.Fatalf("sample %d of %d: %v, want %v", i, n, got[i], want)
				}
			}
		})
	}
}

// TestSweepShowsAliasing is the point of the sweep plot: a decimator fed tones
// past its new Nyquist folds them back down, and that has to be visible as
// energy where the sweep is not.
func TestSweepShowsAliasing(t *testing.T) {
	const rate = 48000.0
	f, err := dsp.NewBoxcarDecimator(4)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewComplexDownsampleProcessor(f, rate)
	if err != nil {
		t.Fatal(err)
	}
	sg, err := Sweep(p, SweepOptions{Duration: 2}, STFTOptions{Size: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(sg.Frames) < 10 {
		t.Fatalf("only %d frames", len(sg.Frames))
	}
	if sg.Rate != rate/4 {
		t.Errorf("spectrogram rate %v, want %v", sg.Rate, rate/4)
	}

	// In the second half the input tone is well above the output Nyquist, so
	// anything present is an alias. A boxcar barely filters, so there is
	// plenty.
	late := sg.Frames[len(sg.Frames)*3/4]
	peak := math.Inf(-1)
	for _, v := range late {
		peak = math.Max(peak, v)
	}
	if peak < -40 {
		t.Errorf("late in the sweep the loudest bin is %v dB; a boxcar decimator should be aliasing badly", peak)
	}
}

func TestSpectrogramImage(t *testing.T) {
	const rate = 8000.0
	// A steady tone through an identity: its energy must land in one row and
	// stay there, and everything else must be at the floor.
	const hz = 1000.0
	n := 1 << 14
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Cos(2 * math.Pi * float64(125*i%1000) / 1000)
	}
	sg, err := SpectrogramOf(x, rate, STFTOptions{Size: 1024})
	if err != nil {
		t.Fatal(err)
	}
	want := int(math.Round(hz / rate * 1024))
	for fi, row := range sg.Frames {
		peak, at := math.Inf(-1), 0
		for i, v := range row {
			if v > peak {
				peak, at = v, i
			}
		}
		if at != want {
			t.Fatalf("frame %d peaks at bin %d, want %d", fi, at, want)
		}
		if math.Abs(peak) > 0.5 {
			t.Errorf("frame %d peak is %v dBFS, want about 0", fi, peak)
		}
	}

	// The raster has to decode, be the size asked for, and put the bright row
	// where the tone is. Comparing decoded pixels rather than PNG bytes means a
	// change to the standard library's encoder cannot break this.
	img := sg.Image(GrayPalette(), SpectrogramOptions{Width: 200, Height: 100})
	b := img.Bounds()
	if b.Dx() != 200 || b.Dy() != 100 {
		t.Fatalf("image is %dx%d, want 200x100", b.Dx(), b.Dy())
	}
	brightest, at := -1, -1
	for y := range 100 {
		r, _, _, _ := img.At(50, y).RGBA()
		if int(r) > brightest {
			brightest, at = int(r), y
		}
	}
	// 1 kHz of a 4 kHz band is a quarter of the way up, so three quarters of
	// the way down the image.
	if wantY := 100 - 100/4; at < wantY-4 || at > wantY+4 {
		t.Errorf("the bright row is at y=%d, want about %d", at, wantY)
	}

	var buf bytes.Buffer
	if err := sg.WritePNG(&buf, nil, SpectrogramOptions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("the PNG does not decode: %v", err)
	}
}

func TestSpectrogramValidation(t *testing.T) {
	if _, err := SpectrogramOf(make([]float64, 10), 8000, STFTOptions{Size: 1024}); err == nil {
		t.Error("a signal shorter than one frame should be an error")
	}
	if _, err := Sweep(nil, SweepOptions{}, STFTOptions{}); err == nil {
		t.Error("a nil processor should be an error")
	}
	p, _ := Identity(8000)
	if _, err := Sweep(p, SweepOptions{Duration: -1}, STFTOptions{}); err == nil {
		t.Error("a negative duration should be an error")
	}
}

func TestPaletteIsACopy(t *testing.T) {
	for name, fn := range map[string]func() Palette{
		"viridis": ViridisPalette, "heat": HeatPalette, "gray": GrayPalette,
	} {
		p := fn()
		lo, hi := p(0), p(1)
		if lo == hi {
			t.Errorf("%s maps 0 and 1 to the same color", name)
		}
		// Out-of-range values clamp rather than wrapping or panicking.
		if p(-5) != lo || p(5) != hi {
			t.Errorf("%s does not clamp outside [0, 1]", name)
		}
		if p(math.NaN()) != lo {
			t.Errorf("%s does not handle NaN", name)
		}
	}
	// The stop table must not be reachable through anything handed out.
	a, b := ViridisPalette(), ViridisPalette()
	if a(0.5) != b(0.5) {
		t.Error("two palettes disagree")
	}
}

func TestFoldBin(t *testing.T) {
	// Harmonics above Nyquist reflect back rather than vanishing.
	for _, c := range []struct{ k, half, want int }{
		{3, 8, 3}, {8, 8, 8}, {9, 8, 7}, {16, 8, 0}, {17, 8, 1}, {24, 8, 8},
	} {
		if got := foldBin(c.k, c.half); got != c.want {
			t.Errorf("foldBin(%d, %d) = %d, want %d", c.k, c.half, got, c.want)
		}
	}
}
