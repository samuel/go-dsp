package plot

import (
	"fmt"

	"samuelks.com/dsp/analysis"
)

// SpectrumChart draws a magnitude spectrum, annotated with the distortion
// figures read off it.
func SpectrumChart(s *analysis.Spectrum, opt ChartOptions) *Chart {
	floor := floorFor(opt.FloorDB, -180, minOf(s.MagDB))

	c := &Chart{
		Subtitle: fmt.Sprintf("%.1f Hz: SFDR %.1f dB, THD %.1f dB, THD+N %.1f dB",
			s.Fundamental, s.SFDR(), s.THD(10), s.THDN()),
		X:      Axis{Label: hzLabel, Min: 0, Max: s.Rate / 2},
		Y:      Axis{Label: "level (dB relative to the drive amplitude)", Min: floorTo(floor, 20), Max: 6},
		Series: []Series{{Name: "spectrum", X: s.Freq, Y: s.MagDB, Width: 1}},
		Notes:  s.Warnings,
	}
	if !opt.Linear {
		// A spectrum is conventionally drawn on a linear frequency axis: the
		// harmonics of a tone are evenly spaced, and that is the pattern the
		// plot exists to show.
		c.X.Log = false
	}
	return opt.apply(c, "Tone spectrum")
}
