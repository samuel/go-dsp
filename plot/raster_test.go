package plot

import (
	"bytes"
	"encoding/xml"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"samuelks.com/dsp/analysis"
)

// TestSpectrogramReducedRasterIsIdentical tests the columns comment: because
// max is associative and idempotent, folding frames into columns and then into
// pixels gives the same pixel as folding every frame into pixels directly.
//
// That holds when the column boundaries line up with the pixel grid, which the
// doubling merge reaches when the frame count is the cap times a power of two --
// so the signal length is chosen to land there.
func TestSpectrogramReducedRasterIsIdentical(t *testing.T) {
	const (
		rate  = 8000
		size  = 128
		hop   = size / 4
		width = 64
	)
	opt := SpectrogramOptions{Width: width, Height: 40, FullScale: true, FloorDB: -120}

	for _, mult := range []int{1, 2, 4, 8, 16} {
		frames := width * mult
		x := testSignal(size+(frames-1)*hop, rate)

		full, err := analysis.SpectrogramOf(x, rate, analysis.STFTOptions{Size: size})
		if err != nil {
			t.Fatal(err)
		}
		if len(full.Frames) != frames {
			t.Fatalf("mult %d: %d frames, want %d", mult, len(full.Frames), frames)
		}
		reduced, err := analysis.SpectrogramOf(x, rate, analysis.STFTOptions{Size: size, Columns: width})
		if err != nil {
			t.Fatal(err)
		}
		if len(reduced.Frames) != width {
			t.Fatalf("mult %d: reduced to %d columns, want exactly %d", mult, len(reduced.Frames), width)
		}

		want := SpectrogramImage(full, GrayPalette(), opt)
		got := SpectrogramImage(reduced, GrayPalette(), opt)
		for y := range 40 {
			for px := range width {
				if got.NRGBAAt(px, y) != want.NRGBAAt(px, y) {
					t.Fatalf("mult %d: pixel (%d,%d) differs from the unreduced raster", mult, px, y)
				}
			}
		}
	}
}

// TestSpectrogramReducedKeepsEveryPeak pins the guarantee that survives when
// the columns do not line up with the pixels: a feature may move by up to a
// column, but none goes missing. Per frequency band, the loudest pixel is as
// loud as unreduced -- a dropped peak is a stopband that is not there.
func TestSpectrogramReducedKeepsEveryPeak(t *testing.T) {
	const (
		rate   = 8000
		width  = 64
		height = 40
	)
	x := testSignal(100000, rate)
	opt := SpectrogramOptions{Width: width, Height: height, FullScale: true, FloorDB: -120}

	full, err := analysis.SpectrogramOf(x, rate, analysis.STFTOptions{Size: 128})
	if err != nil {
		t.Fatal(err)
	}
	want := SpectrogramImage(full, GrayPalette(), opt)

	rowMax := func(img interface {
		NRGBAAt(x, y int) color.NRGBA
	}, y int) uint8 {
		var m uint8
		for px := range width {
			if v := img.NRGBAAt(px, y).R; v > m {
				m = v
			}
		}
		return m
	}

	// Deliberately awkward counts, including fewer columns than pixels.
	for _, cols := range []int{70, 100, 137, 300, 1000} {
		reduced, err := analysis.SpectrogramOf(x, rate, analysis.STFTOptions{Size: 128, Columns: cols})
		if err != nil {
			t.Fatal(err)
		}
		got := SpectrogramImage(reduced, GrayPalette(), opt)
		for y := range height {
			if g, w := rowMax(got, y), rowMax(want, y); g != w {
				t.Errorf("Columns %d: row %d peaks at %d, want %d", cols, y, g, w)
			}
		}
	}
}

// TestTwoSidedChart covers the surround a two-sided spectrogram gets: an axis
// labeled in offsets, the tuner frequency named once, and a log request dropped
// rather than drawn as nonsense.
func TestTwoSidedChart(t *testing.T) {
	const (
		size = 128
		rate = 2.4e6
	)
	x := make([]complex128, 16*size)
	for i := range x {
		th := 2 * math.Pi * float64((7*i)%size) / size
		x[i] = complex(0.5*math.Cos(th), 0.5*math.Sin(th))
	}
	sg, err := analysis.SpectrogramOfComplex(x, rate, analysis.STFTOptions{Size: size, Center: 100e6})
	if err != nil {
		t.Fatal(err)
	}

	c := SpectrogramChart(sg, HeatPalette(), SpectrogramOptions{Title: "capture", LogFreq: true})
	if c.Y.Label != "frequency offset (Hz)" {
		t.Errorf("Y label is %q, want it to say the axis is an offset", c.Y.Label)
	}
	if c.Y.Log {
		t.Error("the frequency axis is logarithmic, which a two-sided spectrum has no version of")
	}
	if c.Y.Min >= 0 {
		t.Errorf("Y starts at %v, want the negative half of the band", c.Y.Min)
	}
	if !strings.Contains(c.Subtitle, "100.000000 MHz") {
		t.Errorf("Subtitle is %q, want the center frequency named", c.Subtitle)
	}
	if !strings.Contains(c.Subtitle, "2.400000 MHz") {
		t.Errorf("Subtitle is %q, want the bandwidth named", c.Subtitle)
	}
	var said bool
	for _, n := range c.Notes {
		if strings.Contains(n, "linear axis") {
			said = true
		}
	}
	if !said {
		t.Errorf("Notes are %q, want one saying the log request was dropped", c.Notes)
	}
	// It still has to render, and to parse as XML.
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(buf.Bytes(), new(struct {
		XMLName xml.Name
	})); err != nil {
		t.Fatalf("the two-sided chart is not well-formed XML: %v", err)
	}

	// A one-sided spectrogram keeps the axis it always had.
	one, err := analysis.SpectrogramOf(testSignal(4096, 8000), 8000, analysis.STFTOptions{Size: 256})
	if err != nil {
		t.Fatal(err)
	}
	oc := SpectrogramChart(one, HeatPalette(), SpectrogramOptions{LogFreq: true})
	if oc.Y.Label != hzLabel {
		t.Errorf("a one-sided chart's Y label is %q, want %q", oc.Y.Label, hzLabel)
	}
	if !oc.Y.Log {
		t.Error("a one-sided chart dropped a valid log axis")
	}
}

func TestHzString(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0 Hz"},
		{440, "440 Hz"},
		{8000, "8.000 kHz"},
		{44100, "44.100 kHz"},
		{2.4e6, "2.400000 MHz"},
		{100e6, "100.000000 MHz"},
		{1.42e9, "1.420000 GHz"},
		{-1000, "-1.000 kHz"},
	} {
		if got := hzString(tc.in); got != tc.want {
			t.Errorf("hzString(%g) = %q, want %q", tc.in, got, tc.want)
		}
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
	sg, err := analysis.SpectrogramOf(x, rate, analysis.STFTOptions{Size: 1024})
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
	img := SpectrogramImage(sg, GrayPalette(), SpectrogramOptions{Width: 200, Height: 100})
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
	if err := WriteSpectrogramPNG(&buf, sg, nil, SpectrogramOptions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("the PNG does not decode: %v", err)
	}
}

// testSignal is a tone, a chirp and a little noise.
func testSignal(n int, rate float64) []float64 {
	r := rand.New(rand.NewPCG(7, 11))
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / rate
		x[i] = 0.4*math.Cos(2*math.Pi*1000*t) +
			0.2*math.Cos(2*math.Pi*(300+400*t)*t) +
			0.01*r.NormFloat64()
	}
	return x
}
