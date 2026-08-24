package dspviz

import (
	"fmt"
	"math"
)

// ChartOptions tunes the standard charts. The zero value is the default for
// every field.
type ChartOptions struct {
	Title         string
	Width, Height int

	// FloorDB clips the magnitude axis from below. Zero selects -120. A request
	// deeper than the measurement can resolve draws noise instead of depth.
	FloorDB float64

	// Linear draws frequency on a linear axis. The default is logarithmic: a
	// linear axis spends more than half its width on the top octave.
	Linear bool
}

func (o ChartOptions) apply(c *Chart, title string) *Chart {
	c.Title = title
	if o.Title != "" {
		c.Title = o.Title
	}
	c.Width, c.Height = o.Width, o.Height
	return c
}

// hzLabel is the x-axis title everywhere, so the charts stack consistently.
const hzLabel = "frequency (Hz)"

// noPoints is the guard for an empty response: no frequency axis to label and
// no curve to draw. FreqResponse already rejects empty frequency lists, so this
// only protects hand-built Responses, as Spectrogram.chart and PSDChart do.
func noPoints(r *Response) bool { return len(r.Freq) == 0 || len(r.H) == 0 }

// MagnitudeChart draws the whole response over a wide decibel range; the zoomed
// views below carve their windows out of it.
func MagnitudeChart(r *Response, opt ChartOptions) *Chart {
	if noPoints(r) {
		return opt.apply(&Chart{}, "Magnitude response")
	}
	floor := floorOr(opt.FloorDB, -120)
	db := r.MagnitudeDB()
	c := &Chart{
		X:      Axis{Label: hzLabel, Log: !opt.Linear, Min: r.Freq[0], Max: r.Freq[len(r.Freq)-1]},
		Y:      Axis{Label: "magnitude (dB)", Min: floor, Max: ceilTo(maxOf(db)+6, 6)},
		Series: []Series{{Name: "magnitude", X: r.Freq, Y: db}},
		Notes:  r.Warnings,
	}
	return opt.apply(c, "Magnitude response")
}

// PassbandChart draws the magnitude over band, scaled to a fraction of a
// decibel so the ripple fills the pane. It only makes sense at this zoom: a
// tenth of a decibel is invisible on an axis that also spans a stopband.
func PassbandChart(r *Response, band [2]float64, opt ChartOptions) *Chart {
	freq, db := within(r.Freq, r.MagnitudeDB(), band)
	lo, hi := minOf(db), maxOf(db)
	if hi-lo < 1e-6 {
		lo, hi = lo-0.01, hi+0.01
	} else {
		pad := (hi - lo) * 0.2
		lo, hi = lo-pad, hi+pad
	}
	c := &Chart{
		Subtitle: fmt.Sprintf("variation %.4f dB peak to peak", maxOf(db)-minOf(db)),
		X:        Axis{Label: hzLabel, Log: !opt.Linear, Min: band[0], Max: band[1]},
		Y:        Axis{Label: "magnitude (dB)", Min: lo, Max: hi},
		Series:   []Series{{Name: "magnitude", X: freq, Y: db}},
		Notes:    r.Warnings,
	}
	if band[0] <= 0 {
		c.X.Log = false
	}
	return opt.apply(c, "Passband")
}

// TransitionChart draws the magnitude around a cutoff over a wide decibel range:
// the transition-width and stopband-depth view. near is the center frequency;
// zero centers on the measured -3 dB point.
func TransitionChart(r *Response, near float64, opt ChartOptions) *Chart {
	if noPoints(r) {
		return opt.apply(&Chart{}, "Transition band")
	}
	db := r.MagnitudeDB()
	if near <= 0 {
		near = Measure(r, [2]float64{r.Freq[0], r.Freq[len(r.Freq)-1] / 4}, [2]float64{}).CutoffHz
	}
	lo, hi := r.Freq[0], r.Freq[len(r.Freq)-1]
	if near > 0 {
		lo = math.Max(lo, near*0.5)
		hi = math.Min(hi, near*2)
	}

	// Scaled to what is actually in the window rather than to the deepest the
	// measurement could show; see floorFor.
	_, visible := within(r.Freq, db, [2]float64{lo, hi})
	floor := floorFor(opt.FloorDB, -180, minOf(visible))

	c := &Chart{
		X:      Axis{Label: hzLabel, Log: !opt.Linear, Min: lo, Max: hi},
		Y:      Axis{Label: "magnitude (dB)", Min: floorTo(floor, 6), Max: ceilTo(maxOf(db)+6, 6)},
		Series: []Series{{Name: "magnitude", X: r.Freq, Y: db}},
		Notes:  r.Warnings,
	}
	if lo <= 0 {
		c.X.Log = false
	}
	return opt.apply(c, "Transition band")
}

// StopbandChart draws the magnitude over band on the deepest scale the
// measurement supports.
func StopbandChart(r *Response, band [2]float64, opt ChartOptions) *Chart {
	freq, db := within(r.Freq, r.MagnitudeDB(), band)

	// As in TransitionChart, the floor is a limit, not a target: a filter that
	// reaches only -70 dB gets an axis that reaches only -70 dB.
	floor := floorFor(opt.FloorDB, -180, minOf(db))

	c := &Chart{
		Subtitle: fmt.Sprintf("peak %.1f dB", maxOf(db)),
		X:        Axis{Label: hzLabel, Log: !opt.Linear, Min: band[0], Max: band[1]},
		Y:        Axis{Label: "magnitude (dB)", Min: floorTo(floor, 6), Max: ceilTo(maxOf(db)+6, 6)},
		Series:   []Series{{Name: "magnitude", X: freq, Y: db}},
		Notes:    r.Warnings,
	}
	if band[0] <= 0 {
		c.X.Log = false
	}
	return opt.apply(c, "Stopband")
}

// PhaseChart draws the phase in degrees.
//
// residual removes the best-fit straight line first, so a linear-phase filter
// reads as flat: pure delay is a straight line in phase and says nothing about a
// filter beyond its latency.
//
// residual needs a passband view. Over a FIR's stopband the amplitude changes
// sign at every null, a real jump of pi that cannot be told from a wrap, so the
// plot is a sawtooth whatever the filter does; the magnitude cutoff below drops
// numerically-nothing points and does not fix that.
func PhaseChart(r *Response, residual bool, opt ChartOptions) *Chart {
	if noPoints(r) {
		return opt.apply(&Chart{}, "Phase")
	}
	var y []float64
	var name, title string
	if residual {
		y, name, title = r.ResidualPhase(), "residual phase", "Phase, linear term removed"
	} else {
		y = r.UnwrappedPhase()
		for i := range y {
			y[i] *= 180 / math.Pi
		}
		name, title = "phase", "Phase"
	}

	// Draw NaN where the magnitude has gone too far down for the phase to mean
	// anything: at a stopband null a linear-phase filter's amplitude changes
	// sign, and the resulting jump of pi looks like phase distortion rather
	// than absence of signal.
	var dropped int
	use := r.significant()
	for i := range y {
		if !use[i] {
			y[i] = math.NaN()
			dropped++
		}
	}

	// Do not zoom into nothing. An exactly linear-phase filter leaves a residual
	// of rounding noise, and auto-ranging over that is a dramatic wander across
	// 1e-13 degrees -- the opposite of what the plot is for. Show a degree
	// minimum.
	yAxis := Axis{Label: "phase (degrees)"}
	if residual {
		lo, hi := minOf(y), maxOf(y)
		if hi-lo < 1 {
			mid := (lo + hi) / 2
			yAxis.Min, yAxis.Max = mid-0.5, mid+0.5
		}
	}

	c := &Chart{
		X:      Axis{Label: hzLabel, Log: !opt.Linear, Min: r.Freq[0], Max: r.Freq[len(r.Freq)-1]},
		Y:      yAxis,
		Series: []Series{{Name: name, X: r.Freq, Y: y}},
		Notes:  r.Warnings,
	}
	if dropped > 0 {
		c.Notes = append(c.Notes, fmt.Sprintf(
			"%d of %d points are more than %g dB below the peak and are left out: the phase of nothing is nothing",
			dropped, len(y), -phaseFloorDB))
	}
	if w := r.GridWarning(); w != "" {
		c.Notes = append(c.Notes, w)
	}
	return opt.apply(c, title)
}

// GroupDelayChart draws the group delay in samples.
func GroupDelayChart(r *Response, opt ChartOptions) *Chart {
	if noPoints(r) {
		return opt.apply(&Chart{}, "Group delay")
	}
	c := &Chart{
		X:      Axis{Label: hzLabel, Log: !opt.Linear, Min: r.Freq[0], Max: r.Freq[len(r.Freq)-1]},
		Y:      Axis{Label: "group delay (samples)"},
		Series: []Series{{Name: "group delay", X: r.Freq, Y: r.GroupDelay()}},
		Notes:  r.Warnings,
	}
	if w := r.GridWarning(); w != "" {
		c.Notes = append(c.Notes, w)
	}
	return opt.apply(c, "Group delay")
}

// ImpulseChart draws an impulse response against time. It shows ringing and,
// in particular, whether it comes before or after the peak: linear-phase
// spreads it evenly on both sides, minimum-phase puts it all after.
func ImpulseChart(h []float64, rate float64, opt ChartOptions) *Chart {
	x := make([]float64, len(h))
	for i := range x {
		x[i] = float64(i)
	}
	peak := 0
	for i, v := range h {
		if math.Abs(v) > math.Abs(h[peak]) {
			peak = i
		}
	}
	c := &Chart{
		Subtitle: fmt.Sprintf("peak at sample %d, %.3f ms", peak, float64(peak)/rate*1000),
		X:        Axis{Label: "sample", Min: 0, Max: math.Max(float64(len(h)-1), 1)},
		Y:        Axis{Label: "amplitude"},
		Series:   []Series{{Name: "impulse", X: x, Y: h, Fill: true}},
	}
	return opt.apply(c, "Impulse response")
}

// StepChart draws a step response against time.
func StepChart(y []float64, rate float64, opt ChartOptions) *Chart {
	x := make([]float64, len(y))
	for i := range x {
		x[i] = float64(i)
	}
	c := &Chart{
		X:      Axis{Label: "sample", Min: 0, Max: math.Max(float64(len(y)-1), 1)},
		Y:      Axis{Label: "amplitude"},
		Series: []Series{{Name: "step", X: x, Y: y}},
	}
	return opt.apply(c, "Step response")
}

// within returns the points of a curve that fall inside band, or the whole
// curve when band is empty.
func within(freq, y []float64, band [2]float64) (fx, fy []float64) {
	if !(band[1] > band[0]) {
		return freq, y
	}
	for i, f := range freq {
		if f >= band[0] && f <= band[1] {
			fx = append(fx, f)
			fy = append(fy, y[i])
		}
	}
	if len(fx) == 0 {
		return freq, y
	}
	return fx, fy
}

func maxOf(v []float64) float64 {
	m := math.Inf(-1)
	for _, x := range v {
		if !math.IsNaN(x) && !math.IsInf(x, 0) {
			m = math.Max(m, x)
		}
	}
	if math.IsInf(m, 0) {
		return 0
	}
	return m
}

func minOf(v []float64) float64 {
	m := math.Inf(1)
	for _, x := range v {
		if !math.IsNaN(x) && !math.IsInf(x, 0) {
			m = math.Min(m, x)
		}
	}
	if math.IsInf(m, 0) {
		return 0
	}
	return m
}

func ceilTo(v, step float64) float64  { return math.Ceil(v/step) * step }
func floorTo(v, step float64) float64 { return math.Floor(v/step) * step }
