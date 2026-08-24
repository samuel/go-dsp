package dspviz

import (
	"math"
	"math/rand/v2"
	"testing"
)

// signalChartSignal is the deterministic recording the new charts are drawn
// from: a tone, a burst and a floor of noise, so every one of them has something
// to show and the golden depends only on the chart code.
func signalChartSignal(n int, rate float64) []float64 {
	r := rand.New(rand.NewPCG(13, 17))
	x := make([]float64, n)
	for i := range x {
		x[i] = 0.02 * r.NormFloat64()
		x[i] += 0.3 * math.Cos(2*math.Pi*float64((64*i)%1024)/1024)
		if i > n/3 && i < n/2 {
			x[i] += 0.5 * math.Cos(2*math.Pi*float64((200*i)%1024)/1024)
		}
	}
	return x
}

// TestSignalChartsGolden pins the new line charts the way the filter charts are
// pinned. There is no golden for anything holding a raster -- see
// TestSpectrogramChartSurround for what is asserted about those instead.
func TestSignalChartsGolden(t *testing.T) {
	const rate = 8000.0
	x := signalChartSignal(1<<16, rate)

	p, err := PSDOf(x, rate, STFTOptions{Size: 1024})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "psd", PSDChart(p, ChartOptions{}))

	w, err := WaveformOf(x, rate, WaveformOptions{Columns: 300})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "waveform", WaveformChart(w, ChartOptions{}))

	l, err := LevelOf(x, rate, LevelOptions{WindowSec: 0.05, Columns: 300})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "level", LevelChart(l, ChartOptions{}))

	h, err := HistogramOf(x, HistogramOptions{Buckets: 512})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "histogram", HistogramChart(h, ChartOptions{}))

	tr, err := ToneTrackOf(x, rate, ToneTrackOptions{
		Mark: 500, Space: 1562.5, Baud: 300, Threshold: 20, Columns: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "tones", ToneTrackChart(tr, ChartOptions{}))
}

// TestSpectrogramChartSurround is what stands in for a golden of a spectrogram.
//
// Chart.WriteSVG is byte-reproducible because every coordinate is written at two
// decimal places and no font metrics enter the layout -- but a spectrogram's
// pane is image/png output, and PNG bytes go through compress/flate, whose
// output has changed between Go releases. So the surround is goldened with the
// raster cleared, and the raster's own properties are asserted directly.
func TestSpectrogramChartSurround(t *testing.T) {
	const rate = 8000.0
	x := signalChartSignal(1<<15, rate)
	sg, err := SpectrogramOf(x, rate, STFTOptions{Size: 512})
	if err != nil {
		t.Fatal(err)
	}
	opt := SpectrogramOptions{Width: 400, Height: 200, FullScale: true, FloorDB: -120}

	c := sg.chart(ViridisPalette(), opt)
	if len(c.Image) == 0 {
		t.Fatal("the chart has no raster in it")
	}
	c.Image = nil
	golden(t, "spectrogram-surround", c)

	img := sg.Image(ViridisPalette(), opt)
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
		t.Errorf("the raster is %dx%d, want 400x200", b.Dx(), b.Dy())
	}
	// The tone is at bin 64 of 1024, which is rate/16, so it lands an eighth of
	// the way up a one-sided pane. That row must be the brightest.
	want := 200 - 1 - int(math.Round((rate/16)/(rate/2)*200))
	brightest, best := 0, -1
	for y := range 200 {
		var sum int
		for x := range 400 {
			r, g, b, _ := img.At(x, y).RGBA()
			sum += int(r + g + b)
		}
		if sum > best {
			brightest, best = y, sum
		}
	}
	if d := brightest - want; d < -3 || d > 3 {
		t.Errorf("the brightest row is %d, want about %d", brightest, want)
	}
}
