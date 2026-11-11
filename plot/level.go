package plot

import (
	"fmt"

	"samuelks.com/dsp/analysis"
)

// LevelChart draws the three curves on a decibel axis.
func LevelChart(l *analysis.Level, opt ChartOptions) *Chart {
	floor := floorFor(opt.FloorDB, -120, minOf(l.RMSDB))

	c := &Chart{
		Subtitle: fmt.Sprintf("%.0f ms windows, peak %.2f dBFS, loudest RMS %.2f dBFS",
			l.WindowSec*1000, maxOf(l.PeakDB), maxOf(l.RMSDB)),
		X: Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(l.Times, l.WindowSec)},
		Y: Axis{Label: "level (dBFS)", Min: floorTo(floor, 10), Max: ceilTo(maxOf(l.PeakDB)+3, 3)},
		Series: []Series{
			{Name: "peak", X: l.Times, Y: l.PeakDB, Width: 1},
			{Name: "rms", X: l.Times, Y: l.RMSDB, Width: 1.5},
			{Name: "crest", X: l.Times, Y: l.CrestDB, Width: 1, Dashed: true},
		},
	}
	return opt.apply(c, "Level against time")
}
