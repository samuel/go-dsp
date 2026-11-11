package plot

import (
	"fmt"
	"slices"

	"samuelks.com/dsp/analysis"
)

// ToneTrackChart draws the two powers and the difference against time, with the
// threshold across them.
//
// The axis is linear and in the detector's own units on purpose. A decibel axis
// would hide the failure this exists for: a threshold set on one scale against a
// signal on another does not look wrong in decibels.
func ToneTrackChart(t *analysis.ToneTrack, opt ChartOptions) *Chart {
	series := []Series{
		{Name: fmt.Sprintf("mark %.0f Hz", t.Mark), X: t.Times, Y: t.MarkPower, Width: 1},
		{Name: fmt.Sprintf("space %.0f Hz", t.Space), X: t.Times, Y: t.SpacePower, Width: 1},
		{Name: "difference", X: t.Times, Y: t.Diff, Width: 1.5},
	}
	if t.Threshold > 0 {
		hi := make([]float64, len(t.Times))
		lo := make([]float64, len(t.Times))
		for i := range hi {
			hi[i], lo[i] = t.Threshold, -t.Threshold
		}
		series = append(series,
			Series{Name: "threshold", X: t.Times, Y: hi, Width: 1, Dashed: true},
			Series{X: t.Times, Y: lo, Width: 1, Dashed: true})
	}

	c := &Chart{
		Subtitle: fmt.Sprintf("%d sample blocks, %d of %d decisive, largest difference %.4g",
			t.Block, t.Crossings, t.Decisions, t.MaxAbsDiff),
		X:      Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(t.Times, float64(t.Block)/t.Rate)},
		Y:      Axis{Label: "Goertzel power"},
		Series: series,
		Notes:  slices.Clone(t.Warnings),
	}
	return opt.apply(c, "Tone pair decision")
}
