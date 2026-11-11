package plot

import (
	"fmt"

	"samuelks.com/dsp/analysis"
)

// HistogramChart draws the distribution on a logarithmic count axis.
//
// A bucket nothing landed in cannot be drawn on a logarithmic axis and leaves a
// gap in the line rather than a point on the floor, which is what makes a comb
// read as a comb.
func HistogramChart(h *analysis.Histogram, opt ChartOptions) *Chart {
	series := []Series{{Name: "count", X: h.Centers, Y: counts(h.Count), Width: 1}}
	if h.Complex {
		series = []Series{
			{Name: "I", X: h.Centers, Y: counts(h.Count), Width: 1},
			{Name: "Q", X: h.Centers, Y: counts(h.CountQ), Width: 1},
			{Name: "|z|", X: h.Centers, Y: counts(h.CountMag), Width: 1},
		}
	}

	var notes []string
	if h.Outside > 0 {
		notes = append(notes, fmt.Sprintf("%d samples are outside full scale and are not counted", h.Outside))
	}
	occupied := h.Occupied()
	if occupied > 0 && occupied < len(h.Count)/2 {
		notes = append(notes, fmt.Sprintf(
			"only %d of %d buckets hold anything: the signal is quantized more coarsely than the file",
			occupied, len(h.Count)))
	}

	c := &Chart{
		Subtitle: fmt.Sprintf("%d samples in %d buckets, from %.4f to %.4f",
			h.Samples, len(h.Count), h.Min, h.Max),
		X:      Axis{Label: "amplitude", Min: -1, Max: 1},
		Y:      Axis{Label: "samples", Log: true},
		Series: series,
		Notes:  notes,
	}
	return opt.apply(c, "Amplitude distribution")
}

func counts(c []int64) []float64 {
	y := make([]float64, len(c))
	for i, v := range c {
		y[i] = float64(v)
	}
	return y
}
