package dspviz

import (
	"math"
	"strconv"
)

// Axis describes one edge of a chart.
//
// Min and Max both zero auto-scales from the data. Ticks nil generates them,
// with NiceTicks or LogTicks as Log selects.
type Axis struct {
	Label    string
	Min, Max float64
	Log      bool
	Ticks    []Tick
}

// Tick is one labeled position on an axis. An empty Label formats Value
// against the spacing of the ticks around it; a Minor tick is drawn but not
// labeled.
type Tick struct {
	Value float64
	Label string
	Minor bool
}

// pos maps a value to a fraction of the axis, 0 at Min and 1 at Max, before any
// clipping. A value outside the range maps outside [0, 1] on purpose: the plot
// clips geometrically so that a line leaving the pane keeps its true slope.
func (a *Axis) pos(v float64) float64 {
	lo, hi := a.Min, a.Max
	if a.Log {
		if v <= 0 || lo <= 0 || hi <= 0 {
			return math.NaN()
		}
		v, lo, hi = math.Log10(v), math.Log10(lo), math.Log10(hi)
	}
	if hi == lo {
		return 0.5
	}
	return (v - lo) / (hi - lo)
}

// autoRange fills Min and Max from the data when the caller left them equal,
// padding a little so the extremes are not drawn on the frame.
func (a *Axis) autoRange(values [][]float64) {
	if a.Min != a.Max {
		return
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, vs := range values {
		for _, v := range vs {
			if math.IsNaN(v) || math.IsInf(v, 0) || (a.Log && v <= 0) {
				continue
			}
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		lo, hi = 0, 1
		if a.Log {
			lo, hi = 1, 10
		}
	}
	switch {
	case lo == hi:
		if a.Log {
			lo, hi = lo/10, hi*10
		} else {
			d := math.Max(math.Abs(lo)*0.05, 1)
			lo, hi = lo-d, hi+d
		}
	case a.Log:
		lo, hi = lo/1.05, hi*1.05
	default:
		pad := (hi - lo) * 0.05
		lo, hi = lo-pad, hi+pad
	}
	a.Min, a.Max = lo, hi
}

func (a *Axis) ticks() []Tick {
	if a.Ticks != nil {
		return a.Ticks
	}
	if a.Log {
		return LogTicks(a.Min, a.Max)
	}
	return NiceTicks(a.Min, a.Max, 6)
}

// NiceTicks returns ticks spanning [lo, hi], stepping by 1, 2 or 5 times a
// power of ten, aiming for about want of them.
//
// The 1-2-5 family is what makes this work unchanged for a decibel axis at
// every zoom the package uses: 0.001, 0.002 and 0.005 are as much a part of it
// as 10 and 20, so a passband plot resolving a hundredth of a decibel gets
// readable ticks from the same code as a stopband plot spanning 180.
func NiceTicks(lo, hi float64, want int) []Tick {
	if want < 2 {
		want = 2
	}
	if !(hi > lo) || math.IsNaN(lo) || math.IsNaN(hi) {
		return []Tick{{Value: lo, Label: strconv.FormatFloat(lo, 'g', 4, 64)}}
	}
	step := niceStep((hi - lo) / float64(want))

	var ticks []Tick
	start := math.Ceil(lo/step) * step
	for i := 0; ; i++ {
		v := start + float64(i)*step
		if v > hi+step*1e-9 {
			break
		}
		if i > 1000 {
			break
		}
		// Snap a value that is a rounding away from zero, so an axis through
		// the origin does not label it "-0.00".
		if math.Abs(v) < step*1e-9 {
			v = 0
		}
		ticks = append(ticks, Tick{Value: v, Label: tickLabel(v, step)})
	}
	return ticks
}

// tickLabel writes a value with just enough decimals to tell it from the tick
// next to it, falling back to exponent form at the extremes. Without that fall
// back, an axis auto-ranged over floating-point noise asks for fourteen decimal
// places and produces labels wider than the chart.
func tickLabel(v, step float64) string {
	decimals := -int(math.Floor(math.Log10(step)))
	if decimals > 6 || math.Abs(v) >= 1e7 {
		return strconv.FormatFloat(v, 'g', 3, 64)
	}
	return strconv.FormatFloat(v, 'f', max(0, decimals), 64)
}

func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsInf(raw, 0) {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch n := raw / mag; {
	case n <= 1:
		return mag
	case n <= 2:
		return 2 * mag
	case n <= 5:
		return 5 * mag
	default:
		return 10 * mag
	}
}

// LogTicks returns decade ticks over [lo, hi], subdivided at 2 and 5.
//
// How the subdivisions are drawn depends on how much ground the axis covers. A
// range of a couple of decades has room to label them, and needs to: a
// transition-band plot spanning half an octave either side of a cutoff contains
// exactly one decade mark, and an axis with one label on it is not an axis.
// Three to five decades draw them unlabelled, as a reading aid. Beyond that
// they are omitted, since a dozen decades of subdivisions is a grey wash.
func LogTicks(lo, hi float64) []Tick {
	if lo <= 0 || hi <= lo {
		return nil
	}
	loDec := int(math.Floor(math.Log10(lo)))
	hiDec := int(math.Ceil(math.Log10(hi)))
	span := hiDec - loDec

	var ticks []Tick
	for d := loDec; d <= hiDec; d++ {
		base := math.Pow(10, float64(d))
		for _, m := range []float64{1, 2, 5} {
			v := base * m
			if v < lo*(1-1e-12) || v > hi*(1+1e-12) {
				continue
			}
			switch {
			case m == 1:
				ticks = append(ticks, Tick{Value: v, Label: logLabel(v)})
			case span <= 2:
				ticks = append(ticks, Tick{Value: v, Label: logLabel(v)})
			case span <= 5:
				ticks = append(ticks, Tick{Value: v, Minor: true})
			}
		}
	}
	return ticks
}

// logLabel writes a decade compactly.
func logLabel(v float64) string {
	switch {
	case v >= 1e6 && math.Mod(v, 1e6) == 0:
		return strconv.FormatFloat(v/1e6, 'g', -1, 64) + "M"
	case v >= 1e3 && math.Mod(v, 1e3) == 0:
		return strconv.FormatFloat(v/1e3, 'g', -1, 64) + "k"
	case v >= 1:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return strconv.FormatFloat(v, 'g', 2, 64)
	}
}

// timeAxisEnd returns where a time axis ends, given the start times of its
// columns and a width to fall back on when there are too few to measure one.
//
// Each Times[i] is the start of a column, not its center, so the axis has to
// run one column past the last of them. Ending it at the last start time
// squeezes the final column onto the right edge, and for a spectrogram -- whose
// raster fills the pane whatever the axis says -- it puts every column against
// a time that is short by one, which is a whole column of drift by the end.
//
// The width is measured rather than taken from the hop, because the fold
// reducer widens columns as it goes and the caller does not know by how much.
func timeAxisEnd(times []float64, fallback float64) float64 {
	switch len(times) {
	case 0:
		return fallback
	case 1:
		return times[0] + fallback
	}
	last := times[len(times)-1]
	return last + (last-times[0])/float64(len(times)-1)
}
