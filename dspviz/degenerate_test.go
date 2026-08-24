package dspviz

import (
	"bytes"
	"io"
	"testing"
)

// TestDegenerateSpectrumInputs feeds every reader of a Spectrogram and a PSD
// one with no bins and one with a single bin.
//
// NewSpectrogramBuilder rejects the frame size that produces either, so it
// takes a hand-built value to get here -- which is exactly why this is worth
// pinning: the guards in Image and the two charts are the only thing standing
// between a struct literal and an index out of range, and nothing in the
// ordinary path exercises them.
func TestDegenerateSpectrumInputs(t *testing.T) {
	for _, bins := range []int{0, 1} {
		name := "nobins"
		if bins == 1 {
			name = "onebin"
		}
		t.Run(name, func(t *testing.T) {
			p := &PSD{Rate: 8000, Frames: 1, Floor: -180, NoiseBW: 1}
			sg := &Spectrogram{Rate: 8000, Floor: -180}
			if bins == 1 {
				p.Freq = []float64{0}
				p.AvgDB = []float64{-10}
				p.MaxDB = []float64{-10}
				p.MinDB = []float64{-10}
				sg.Freqs = []float64{0}
				sg.Times = []float64{0}
				sg.Frames = [][]float64{{-10}}
			}
			for _, step := range []struct {
				name string
				fn   func()
			}{
				{"PSD.PeakBin", func() { p.PeakBin(0) }},
				{"PSD.NoiseFloorDB", func() { p.NoiseFloorDB(20) }},
				{"PSD.MeanSquare", func() { p.MeanSquare() }},
				{"OccupiedBandwidth", func() { OccupiedBandwidth(p, 0.99) }},
				{"SpectralStatsOf", func() { SpectralStatsOf(p) }},
				{"PSDChart", func() { mustWriteSVG(t, PSDChart(p, ChartOptions{})) }},
				{"Spectrogram.Image", func() { sg.Image(nil, SpectrogramOptions{}) }},
				{"Spectrogram.WriteSVG", func() {
					if err := sg.WriteSVG(io.Discard, nil, SpectrogramOptions{}); err != nil {
						t.Error(err)
					}
				}},
				// The bare raster is the one reader that reports rather than
				// drawing nothing: Image has no frequency axis to work with, so
				// it hands back a 0x0 image and image/png will not encode one.
				// An error is the contract here; a panic is not.
				{"Spectrogram.WritePNG", func() {
					if err := sg.WritePNG(&bytes.Buffer{}, nil, SpectrogramOptions{}); err == nil {
						t.Error("WritePNG on a spectrogram with no frequency axis returned no error")
					}
				}},
			} {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s panicked: %v", step.name, r)
						}
					}()
					step.fn()
				}()
			}
		})
	}
}

// TestDegenerateChartInputs is the same for the charts drawn from a recording,
// each handed the zero value of what it draws. A run over a capture too short
// for one frame reaches these.
func TestDegenerateChartInputs(t *testing.T) {
	for _, c := range []struct {
		name string
		make func() *Chart
	}{
		{"SpectrumChart", func() *Chart { return SpectrumChart(&Spectrum{Rate: 8000}, ChartOptions{}) }},
		{"WaveformChart", func() *Chart { return WaveformChart(&Waveform{Rate: 8000}, ChartOptions{}) }},
		{"LevelChart", func() *Chart { return LevelChart(&Level{Rate: 8000}, ChartOptions{}) }},
		{"HistogramChart", func() *Chart { return HistogramChart(&Histogram{}, ChartOptions{}) }},
		{"ToneTrackChart", func() *Chart { return ToneTrackChart(&ToneTrack{Rate: 8000}, ChartOptions{}) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			mustWriteSVG(t, c.make())
		})
	}
}

// mustWriteSVG renders a chart and requires the result to be well-formed XML,
// which is the only thing an empty chart still has to get right.
func mustWriteSVG(t *testing.T, c *Chart) {
	t.Helper()
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}
	if err := parseXML(buf.Bytes()); err != nil {
		t.Fatalf("not well-formed XML: %v", err)
	}
}

// TestDegenerateResponse feeds every reader of a Response one with no points
// and one with a single point. FreqResponse rejects an empty frequency list, so
// these are hand-built -- which is the case the group delay and the phase
// unwrap have to survive without indexing a neighbor that is not there.
//
// Freq and H are always the same length: every Response comes out of
// FreqResponse, which sets both. A pair that disagrees is not covered here,
// because guarding every reader against inconsistent state would be a check on
// this package rather than on its input.
func TestDegenerateResponse(t *testing.T) {
	for _, r := range []*Response{
		{Rate: 8000},
		{Rate: 8000, Freq: []float64{100}, H: []complex128{complex(0.5, 0.5)}},
		// A zero rate, which divides into every angular frequency.
		{Freq: []float64{100, 200}, H: []complex128{1, 1}},
	} {
		for _, step := range []struct {
			name string
			fn   func()
		}{
			{"MagnitudeDB", func() { r.MagnitudeDB() }},
			{"Phase", func() { r.Phase() }},
			{"UnwrappedPhase", func() { r.UnwrappedPhase() }},
			{"ResidualPhase", func() { r.ResidualPhase() }},
			{"GroupDelay", func() { r.GroupDelay() }},
			{"GroupDelaySeconds", func() { r.GroupDelaySeconds() }},
			{"GridWarning", func() { r.GridWarning() }},
			{"MagnitudeChart", func() { mustWriteSVG(t, MagnitudeChart(r, ChartOptions{})) }},
			{"PhaseChart", func() { mustWriteSVG(t, PhaseChart(r, false, ChartOptions{})) }},
			{"GroupDelayChart", func() { mustWriteSVG(t, GroupDelayChart(r, ChartOptions{})) }},
			{"TransitionChart", func() { mustWriteSVG(t, TransitionChart(r, 0, ChartOptions{})) }},
			{"PassbandChart", func() {
				mustWriteSVG(t, PassbandChart(r, [2]float64{20, 500}, ChartOptions{}))
			}},
			{"StopbandChart", func() {
				mustWriteSVG(t, StopbandChart(r, [2]float64{4000, 8000}, ChartOptions{}))
			}},
			{"Measure", func() { Measure(r, [2]float64{20, 500}, [2]float64{4000, 8000}) }},
		} {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("%d points, %d H: %s panicked: %v",
							len(r.Freq), len(r.H), step.name, rec)
					}
				}()
				step.fn()
			}()
		}
	}
}
