package plot

import (
	"fmt"
	"math"

	"samuelks.com/dsp/analysis"
)

// zigzag interleaves an envelope into the single alternating series a waveform
// is drawn as: minimum then maximum for each column, so one path covers the
// whole band the samples occupied. Two filled series instead would draw the
// area between them as though the signal were never there, which is the
// opposite of the truth.
func zigzag(times, mins, maxs []float64) (xs, ys []float64) {
	n := min(len(times), min(len(mins), len(maxs)))
	xs = make([]float64, 0, 2*n)
	ys = make([]float64, 0, 2*n)
	for i := range n {
		xs = append(xs, times[i], times[i])
		ys = append(ys, mins[i], maxs[i])
	}
	return xs, ys
}

// WaveformChart draws the envelope against time.
func WaveformChart(w *analysis.Waveform, opt ChartOptions) *Chart {
	xs, ys := zigzag(w.Times, w.Min, w.Max)
	peak := math.Max(maxOf(w.Max), -minOf(w.Min))
	span := math.Max(peak*1.05, 1e-6)

	c := &Chart{
		Subtitle: fmt.Sprintf("%d samples in %d columns, peak %.4f (%.2f dBFS)",
			w.Samples, len(w.Times), peak, analysis.DB(peak)),
		X:      Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(w.Times, 1/w.Rate)},
		Y:      Axis{Label: "amplitude", Min: -span, Max: span},
		Series: []Series{{Name: "waveform", X: xs, Y: ys, Width: 1}},
		Notes: []string{
			"each column is the minimum and the maximum of the samples in it, never their mean: " +
				"a transient one sample wide is what this plot is for",
		},
	}
	return opt.apply(c, "Waveform")
}
