package dspviz

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Report is everything the standard set of views needs, measured once. The
// charts are four windows onto one response plus two time-domain plots and two
// driven measurements, so measuring them separately would be both slower and
// liable to disagree with itself.
type Report struct {
	Rates   [2]float64
	Bands   [2][2]float64 // passband and stopband, in Hz
	Metrics Metrics

	Response *Response
	Impulse  []float64
	Step     []float64
	Spectrum *Spectrum
	Sweep    *Spectrogram

	Warnings []string
}

// ReportOptions selects how much work Analyze does.
type ReportOptions struct {
	Points   int     // frequency grid size; 0 selects 2048
	Frame    int     // measurement frame; 0 selects the package default
	Taps     int     // impulse and step length; 0 selects 128
	ToneHz   float64 // 0 selects 997
	SweepSec float64 // 0 selects 8; negative skips the sweep
	STFTSize int
	SkipTone bool

	// Passband and Stopband override the bands Analyze would otherwise guess.
	// A caller that designed the filter knows where they are; the guess below
	// only exists for one that does not, and it is conservative enough to
	// understate a ripple by leaving the band edge out of the band.
	Passband, Stopband [2]float64
}

// Analyze measures everything the charts need from one processor.
//
// The bands are worked out from the measurement rather than being asked for:
// the passband runs from the bottom of the grid to the measured cutoff, and the
// stopband from twice the cutoff to the top. That is a guess, and a caller who
// knows better should set Report.Bands and call Measure again -- but it is a
// good enough guess to label a plot with, and it means the common case needs no
// arguments at all.
func Analyze(p Processor, opt ReportOptions) (*Report, error) {
	if p == nil {
		return nil, errors.New("dspviz: nil processor")
	}
	inRate, outRate := p.Rates()
	if !(inRate > 0) || !(outRate > 0) {
		// LogGrid below panics on a non-positive bound, and every level this
		// report reports is relative to a rate. A Processor is free to be
		// written outside this package, so this is reachable without a bug
		// here.
		return nil, fmt.Errorf("dspviz: the processor reports rates of %g in and %g out; both have to be positive", inRate, outRate)
	}

	points := opt.Points
	if points <= 0 {
		points = 2048
	}
	taps := opt.Taps
	if taps <= 0 {
		taps = 128
	}
	tone := opt.ToneHz
	if tone == 0 {
		tone = 997
	}

	nyq := math.Min(inRate, outRate) / 2
	low := math.Max(nyq/10000, 1)
	r, err := FreqResponse(p, LogGrid(low, nyq*0.999, points), ResponseOptions{Frame: opt.Frame})
	if err != nil {
		return nil, fmt.Errorf("dspviz: measuring the response: %w", err)
	}

	rep := &Report{
		Rates:    [2]float64{inRate, outRate},
		Response: r,
		Impulse:  Impulse(p, taps),
		Step:     Step(p, taps),
		Warnings: r.Warnings,
	}

	switch {
	case opt.Passband[1] > opt.Passband[0]:
		rep.Bands = [2][2]float64{opt.Passband, opt.Stopband}
	default:
		// A first pass with the whole band as passband finds the cutoff, and
		// the bands follow from it.
		cut := Measure(r, [2]float64{low, nyq}, [2]float64{}).CutoffHz
		if cut <= 0 || cut >= nyq*0.98 {
			rep.Bands = [2][2]float64{{low, nyq * 0.999}, {}}
		} else {
			rep.Bands = [2][2]float64{{low, cut * 0.8}, {math.Min(cut*2, nyq*0.9), nyq * 0.999}}
		}
	}
	rep.Metrics = Measure(r, rep.Bands[0], rep.Bands[1])

	if !opt.SkipTone {
		s, err := ToneSpectrum(p, tone, SpectrumOptions{Frame: opt.Frame})
		if err != nil {
			rep.Warnings = append(rep.Warnings, "tone spectrum: "+err.Error())
		} else {
			rep.Spectrum = s
		}
	}
	if opt.SweepSec >= 0 {
		sg, err := Sweep(p, SweepOptions{Duration: opt.SweepSec}, STFTOptions{Size: opt.STFTSize})
		if err != nil {
			rep.Warnings = append(rep.Warnings, "sweep: "+err.Error())
		} else {
			rep.Sweep = sg
		}
	}
	return rep, nil
}

// Charts returns the standard set, keyed by the name each is written under.
// A chart that would say nothing about this processor is left out rather than
// drawn empty: there is no stopband plot for a filter with no stopband.
func (r *Report) Charts(opt ChartOptions) map[string]*Chart {
	m := map[string]*Chart{
		"magnitude":  MagnitudeChart(r.Response, opt),
		"passband":   PassbandChart(r.Response, r.Bands[0], opt),
		"phase":      PhaseChart(r.Response, false, opt),
		"groupdelay": GroupDelayChart(r.Response, opt),
		"impulse":    ImpulseChart(r.Impulse, r.Rates[1], opt),
		"step":       StepChart(r.Step, r.Rates[1], opt),
	}
	if r.Metrics.CutoffHz > 0 {
		m["transition"] = TransitionChart(r.Response, r.Metrics.CutoffHz, opt)
	}
	if r.Bands[1][1] > r.Bands[1][0] {
		m["stopband"] = StopbandChart(r.Response, r.Bands[1], opt)
	}
	if r.Spectrum != nil {
		m["tonespectrum"] = SpectrumChart(r.Spectrum, opt)
	}
	return m
}

// WriteSummary prints the numbers as a short table.
func (r *Report) WriteSummary(w io.Writer) error {
	var b strings.Builder
	m := r.Metrics
	fmt.Fprintf(&b, "rate          %.0f Hz in, %.0f Hz out\n", r.Rates[0], r.Rates[1])
	fmt.Fprintf(&b, "method        %v\n", r.Response.Method)
	if m.CutoffHz > 0 {
		fmt.Fprintf(&b, "cutoff        %.2f Hz (-3 dB)\n", m.CutoffHz)
	}
	if m.TransitionHz > 0 {
		fmt.Fprintf(&b, "transition    %.1f Hz wide, to %.1f Hz\n", m.TransitionHz, m.StopEdgeHz)
	}
	if m.Passband.Points > 0 {
		fmt.Fprintf(&b, "passband      %.0f-%.0f Hz, %.4f dB peak to peak\n",
			r.Bands[0][0], r.Bands[0][1], m.Passband.RippleDB)
	}
	if m.Stopband.Points > 0 {
		fmt.Fprintf(&b, "stopband      %.0f-%.0f Hz, peak %.1f dB\n",
			r.Bands[1][0], r.Bands[1][1], m.Stopband.MaxDB)
	}
	fmt.Fprintf(&b, "group delay   %.3f samples mean over the passband\n", m.GroupDelaySamples)
	if s := r.Spectrum; s != nil {
		fmt.Fprintf(&b, "at %.0f Hz     SFDR %.1f dB, THD %.1f dB, THD+N %.1f dB\n",
			s.Fundamental, s.SFDR(), s.THD(10), s.THDN())
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "note          %s\n", w)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
