package dspviz

import (
	"bytes"
	"encoding/xml"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

// testSignal is a deterministic signal with something in it at every scale: a
// tone, a slow sweep and noise, so a spectrogram of it has real structure rather
// than one bright line.
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

// TestSpectrogramStreamingMatchesWholeSignal is the guarantee the refactor has to make:
// the same samples through SpectrogramOf and through a builder fed in arbitrary pieces
// produce the same numbers. Exactly the same -- the same additions in the same
// order -- so equality is the right comparison and any drift is a bug rather
// than rounding.
func TestSpectrogramStreamingMatchesWholeSignal(t *testing.T) {
	const rate = 8000
	x := testSignal(5000, rate)
	opt := STFTOptions{Size: 256}

	want, err := SpectrogramOf(x, rate, opt)
	if err != nil {
		t.Fatal(err)
	}

	hop := 256 / 4
	for _, block := range []int{1, 2, 3, hop - 1, hop, hop + 1, 255, 256, 257, 1000, len(x)} {
		b, err := NewSpectrogramBuilder(rate, opt)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(x); i += block {
			if err := b.Write(x[i:min(i+block, len(x))]); err != nil {
				t.Fatal(err)
			}
		}
		got, err := b.Close()
		if err != nil {
			t.Fatal(err)
		}

		if len(got.Frames) != len(want.Frames) {
			t.Fatalf("block %d: %d frames, want %d", block, len(got.Frames), len(want.Frames))
		}
		if got.Peak != want.Peak {
			t.Errorf("block %d: Peak = %v, want %v", block, got.Peak, want.Peak)
		}
		if !slices.Equal(got.Times, want.Times) {
			t.Errorf("block %d: Times differ", block)
		}
		if !slices.Equal(got.Freqs, want.Freqs) {
			t.Errorf("block %d: Freqs differ", block)
		}
		for f := range want.Frames {
			if !slices.Equal(got.Frames[f], want.Frames[f]) {
				t.Fatalf("block %d: frame %d differs from the whole-signal result", block, f)
			}
		}
	}
}

// TestSpectrogramComplexStreamingMatches is the same guarantee for the I/Q path.
func TestSpectrogramComplexStreamingMatches(t *testing.T) {
	const rate = 8000
	x := make([]complex128, 4000)
	r := rand.New(rand.NewPCG(3, 5))
	for i := range x {
		th := 2 * math.Pi * 1300 * float64(i) / rate
		x[i] = complex(0.5*math.Cos(th)+0.01*r.NormFloat64(), 0.5*math.Sin(th)+0.01*r.NormFloat64())
	}
	opt := STFTOptions{Size: 256}

	want, err := SpectrogramOfComplex(x, rate, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range []int{1, 7, 64, 255, 256, 257, 999} {
		b, err := NewSpectrogramBuilder(rate, STFTOptions{Size: 256, Complex: true})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(x); i += block {
			if err := b.WriteComplex(x[i:min(i+block, len(x))]); err != nil {
				t.Fatal(err)
			}
		}
		got, err := b.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Frames) != len(want.Frames) || got.Peak != want.Peak {
			t.Fatalf("block %d: %d frames peak %v, want %d frames peak %v",
				block, len(got.Frames), got.Peak, len(want.Frames), want.Peak)
		}
		for f := range want.Frames {
			if !slices.Equal(got.Frames[f], want.Frames[f]) {
				t.Fatalf("block %d: frame %d differs", block, f)
			}
		}
	}
}

// TestSpectrogramFullScale is the scaling contract, and it is what a wrong window gain
// or a misplaced factor of two shows up in. A sine at full scale reads 0 dBFS,
// coherently sampled so all its energy is in one bin.
func TestSpectrogramFullScale(t *testing.T) {
	const (
		size = 1024
		rate = 8000.0
		bin  = 64
	)
	x := make([]float64, 8*size)
	for i := range x {
		x[i] = math.Cos(2 * math.Pi * float64((bin*i)%size) / size)
	}
	sg, err := SpectrogramOf(x, rate, STFTOptions{Size: size, Window: rectangular})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sg.Peak) > 1e-9 {
		t.Errorf("a full-scale sine peaked at %v dBFS, want 0", sg.Peak)
	}
	if got := sg.Frames[0][bin]; math.Abs(got) > 1e-9 {
		t.Errorf("bin %d reads %v dBFS, want 0", bin, got)
	}
	if got := sg.Freqs[bin]; math.Abs(got-bin*rate/size) > 1e-9 {
		t.Errorf("bin %d is labeled %v Hz, want %v", bin, got, bin*rate/size)
	}
}

// TestSpectrogramComplexFullScale is the factor-of-two test, and the reason it is
// written out separately from the real one.
//
// A real sine's energy is split between a bin and its mirror, so the magnitude is
// doubled to account for the half that is not kept. A complex exponential has no
// mirror -- its negative frequency is a bin of its own -- so doubling would
// report every I/Q signal 6.02 dB hot, and the picture would look perfectly
// reasonable while every number on the color bar was wrong.
func TestSpectrogramComplexFullScale(t *testing.T) {
	const (
		size = 1024
		rate = 8000.0
	)
	for _, bin := range []int{-256, -1, 0, 1, 200} {
		x := make([]complex128, 8*size)
		for i := range x {
			th := 2 * math.Pi * float64((((bin%size)+size)%size*i)%size) / size
			x[i] = complex(math.Cos(th), math.Sin(th))
		}
		sg, err := SpectrogramOfComplex(x, rate, STFTOptions{Size: size, Window: rectangular})
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(sg.Peak) > 1e-9 {
			t.Errorf("bin %d: a full-scale exponential peaked at %v dBFS, want 0", bin, sg.Peak)
		}
		// And in exactly one bin, the one the two-sided layout says.
		row := sg.Frames[0]
		want := bin + size/2
		loud := 0
		for i := range row {
			if row[i] > -100 {
				loud++
			}
		}
		if loud != 1 {
			t.Errorf("bin %d: %d bins are above -100 dBFS, want 1", bin, loud)
		}
		if math.Abs(row[want]) > 1e-9 {
			t.Errorf("bin %d: row[%d] = %v dBFS, want 0", bin, want, row[want])
		}
		if got := sg.Freqs[want]; math.Abs(got-float64(bin)*rate/size) > 1e-9 {
			t.Errorf("bin %d is labeled %v Hz, want %v", bin, got, float64(bin)*rate/size)
		}
	}
}

func rectangular(w []float64) {
	for i := range w {
		w[i] = 1
	}
}

// TestSpectrogramTwoSidedFreqs checks the axis a two-sided spectrogram advertises.
func TestSpectrogramTwoSidedFreqs(t *testing.T) {
	const (
		size = 64
		rate = 8000.0
	)
	sg, err := SpectrogramOfComplex(make([]complex128, size), rate, STFTOptions{Size: size, Center: 100e6})
	if err != nil {
		t.Fatal(err)
	}
	if !sg.TwoSided() {
		t.Fatal("a complex spectrogram does not report itself two-sided")
	}
	if len(sg.Freqs) != size {
		t.Errorf("%d bins, want %d: a complex spectrum keeps all of them", len(sg.Freqs), size)
	}
	if got, want := sg.Freqs[0], -rate/2; got != want {
		t.Errorf("the first bin is %v Hz, want %v", got, want)
	}
	if got := sg.Freqs[size/2]; got != 0 {
		t.Errorf("the middle bin is %v Hz, want DC", got)
	}
	if got, want := sg.Freqs[size-1], rate/2-rate/size; got != want {
		t.Errorf("the last bin is %v Hz, want %v", got, want)
	}
	if !slices.IsSorted(sg.Freqs) {
		t.Error("the frequencies are not monotonic, so the shift did not happen")
	}
	if sg.Center != 100e6 {
		t.Errorf("Center = %v, want it carried through", sg.Center)
	}

	// A real spectrogram is one-sided and says so.
	realSpec, err := SpectrogramOf(make([]float64, size), rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	if realSpec.TwoSided() {
		t.Error("a real spectrogram reports itself two-sided")
	}
	if len(realSpec.Freqs) != size/2+1 {
		t.Errorf("%d bins, want %d", len(realSpec.Freqs), size/2+1)
	}
}

// TestSpectrogramWrongShape keeps the two write methods from being interchangeable,
// since silently taking the real part of an I/Q stream would be a plausible
// looking wrong answer.
func TestSpectrogramWrongShape(t *testing.T) {
	b, _ := NewSpectrogramBuilder(8000, STFTOptions{Size: 64, Complex: true})
	if err := b.Write(make([]float64, 64)); err == nil {
		t.Error("Write on a two-sided builder succeeded")
	}
	r, _ := NewSpectrogramBuilder(8000, STFTOptions{Size: 64})
	if err := r.WriteComplex(make([]complex128, 64)); err == nil {
		t.Error("WriteComplex on a one-sided builder succeeded")
	}
	if _, err := SpectrogramOf(make([]float64, 10), 8000, STFTOptions{Size: 1024}); err == nil {
		t.Error("a signal shorter than one frame produced a spectrogram")
	}
	if _, err := NewSpectrogramBuilder(0, STFTOptions{}); err == nil {
		t.Error("a rate of zero was accepted")
	}
}

// TestSpectrogramHop covers hops that are not the default, including one wider than the
// frame, where whole stretches of the signal are covered by no frame at all.
func TestSpectrogramHop(t *testing.T) {
	const (
		size = 64
		rate = 1000.0
	)
	x := testSignal(1000, rate)
	for _, hop := range []int{1, 7, 32, 63, 64, 65, 100, 256} {
		sg, err := SpectrogramOf(x, rate, STFTOptions{Size: size, Hop: hop})
		if err != nil {
			t.Fatal(err)
		}
		// The frames are those starting at 0, hop, 2*hop ... that still fit.
		want := 0
		for start := 0; start+size <= len(x); start += hop {
			want++
		}
		if len(sg.Frames) != want {
			t.Errorf("hop %d: %d frames, want %d", hop, len(sg.Frames), want)
		}
		for i, tm := range sg.Times {
			if got, exp := tm, float64(i*hop)/rate; math.Abs(got-exp) > 1e-12 {
				t.Errorf("hop %d: Times[%d] = %v, want %v", hop, i, got, exp)
				break
			}
		}

		// And feeding it in pieces has to agree, which is where a hop wider than
		// the frame gets its discarding wrong if it is going to.
		b, _ := NewSpectrogramBuilder(rate, STFTOptions{Size: size, Hop: hop})
		for i := 0; i < len(x); i += 13 {
			if err := b.Write(x[i:min(i+13, len(x))]); err != nil {
				t.Fatal(err)
			}
		}
		got, err := b.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Frames) != len(sg.Frames) {
			t.Fatalf("hop %d: streamed %d frames, whole-signal gave %d", hop, len(got.Frames), len(sg.Frames))
		}
		for f := range sg.Frames {
			if !slices.Equal(got.Frames[f], sg.Frames[f]) {
				t.Fatalf("hop %d: streamed frame %d differs", hop, f)
			}
		}
	}
}

// TestFrameSize checks the sizes a caller gets for real rates, and that they are
// the nearest power of two rather than the next one up -- rounding 1200 up to
// 2048 would silently ask for almost double the frame.
func TestFrameSize(t *testing.T) {
	for _, tc := range []struct {
		rate, seconds float64
		want          int
	}{
		{48000, 0.025, 1024},      // 1200 -> 1024, the nearer side
		{44100, 0.025, 1024},      // 1102
		{11025, 0.025, 256},       // 276
		{8000, 0.025, 256},        // 200 -> 256
		{2400000, 0.025, 1 << 16}, // 60000 -> clamped
		{100, 0.025, 256},         // 2.5 -> clamped up
		{48000, 1, 1 << 16},       // clamped
		{0, 0.025, 2048},          // no rate to work from
		{48000, 0, 2048},
	} {
		if got := FrameSize(tc.rate, tc.seconds); got != tc.want {
			t.Errorf("FrameSize(%g, %g) = %d, want %d", tc.rate, tc.seconds, got, tc.want)
		}
	}

	// Every answer is a usable transform length.
	for _, rate := range []float64{8000, 11025, 22050, 44100, 48000, 96000, 250000, 2048000} {
		n := FrameSize(rate, 0.025)
		if n&(n-1) != 0 {
			t.Errorf("FrameSize(%g) = %d, not a power of two", rate, n)
		}
		if _, err := dsp.NewFFT[complex128](n); err != nil {
			t.Errorf("FrameSize(%g) = %d, which NewFFT rejects: %v", rate, n, err)
		}
	}
}

// TestFrameSizeResolvesKeying is the reason FrameSize exists rather than a
// constant.
func TestFrameSizeResolvesKeying(t *testing.T) {
	const rate = 11025.0
	const baud = 300.0

	size := FrameSize(rate, 0.025)
	frame := float64(size) / rate
	symbol := 1 / baud
	if frame > 8*symbol {
		t.Errorf("a %d point frame is %.1f ms against a %.1f ms symbol; the keying is averaged away",
			size, frame*1000, symbol*1000)
	}
	// And the bins still have to separate a 200 Hz shift.
	if binHz := rate / float64(size); binHz > 100 {
		t.Errorf("%d point frames give %.1f Hz bins, too coarse for a 200 Hz shift", size, binHz)
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
	sg, err := SpectrogramOfComplex(x, rate, STFTOptions{Size: size, Center: 100e6})
	if err != nil {
		t.Fatal(err)
	}

	c := SweepChart(sg, HeatPalette(), SpectrogramOptions{Title: "capture", LogFreq: true})
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
	one, err := SpectrogramOf(testSignal(4096, 8000), 8000, STFTOptions{Size: 256})
	if err != nil {
		t.Fatal(err)
	}
	oc := SweepChart(one, HeatPalette(), SpectrogramOptions{LogFreq: true})
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
