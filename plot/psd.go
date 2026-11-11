package plot

import (
	"fmt"
	"math"
	"slices"

	"samuelks.com/dsp/analysis"
)

// PSDChart draws the three curves, with the estimated noise floor as a dashed
// line across them.
func PSDChart(p *analysis.PSD, opt ChartOptions) *Chart {
	if len(p.Freq) == 0 {
		// No frequency axis to label. NewSpectrogramBuilder rejects the frame
		// size that produces this, so it takes a hand-built PSD to get here;
		// an empty chart beats indexing Freq[0].
		return opt.apply(&Chart{}, "Averaged spectrum")
	}
	floor := floorOr(opt.FloorDB, math.Max(p.Floor, minOf(p.MinDB)-6))
	noise := p.NoiseFloorDB(20)
	flat := make([]float64, len(p.Freq))
	for i := range flat {
		flat[i] = noise
	}

	lo, hi := analysis.OccupiedBandwidth(p, 0.99)
	c := &Chart{
		Subtitle: fmt.Sprintf("%d frames, %.2f Hz of noise bandwidth per bin, noise floor %.1f dBFS, 99%% of the power in %.0f Hz",
			p.Frames, p.NoiseBW, noise, hi-lo),
		X: Axis{Label: hzLabel, Min: p.Freq[0], Max: p.Freq[len(p.Freq)-1]},
		Y: Axis{Label: "level (dBFS per bin)", Min: floorTo(floor, 10), Max: ceilTo(maxOf(p.MaxDB)+6, 10)},
		Series: []Series{
			{Name: "peak", X: p.Freq, Y: p.MaxDB, Width: 1},
			{Name: "average", X: p.Freq, Y: p.AvgDB, Width: 1.5},
			{Name: "minimum", X: p.Freq, Y: p.MinDB, Width: 1},
			{Name: "noise floor", X: p.Freq, Y: flat, Width: 1, Dashed: true},
		},
		Notes: append(slices.Clone(p.Warnings),
			fmt.Sprintf("levels are per bin, not per hertz; one bin is %.3f Hz of noise bandwidth", p.NoiseBW)),
	}
	if p.TwoSided() {
		c.X.Label = "frequency offset (Hz)"
	}
	title := "Averaged spectrum"
	if p.Center != 0 {
		title = "Averaged spectrum, center " + hzString(p.Center)
	}
	return opt.apply(c, title)
}
