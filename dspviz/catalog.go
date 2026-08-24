package dspviz

import (
	"fmt"
	"math"
	"slices"

	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/firpm"
)

// Param describes one adjustable number of a filter kind, with enough
// information for a caller to build a control for it without knowing what the
// filter is.
type Param struct {
	Name    string  `json:"name"`
	Label   string  `json:"label"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Step    float64 `json:"step"`
	Log     bool    `json:"log"` // the useful range spans decades, so a control should too

	// Integer says the value is a count rather than a measurement: Step is the
	// spacing of a real grid, and Build snaps the value onto Min + k*Step.
	//
	// The two discrete parameters here have grids that are part of the filter.
	// Taps steps by two from three, since an even tap count is a Type II design
	// with a forced null at Nyquist rather than the filter the catalog
	// describes; a decimation factor has no meaning between whole numbers. A
	// rate is not one of these: snapping 22050 Hz to a multiple of 100 would
	// change the ratio a rational decimator was asked for.
	Integer bool   `json:"integer,omitempty"`
	Unit    string `json:"unit,omitempty"`
}

// snap puts v on the grid this parameter advertises, clamped to its range.
func (p Param) snap(v float64) float64 {
	v = math.Max(p.Min, math.Min(p.Max, v))
	if !p.Integer {
		return v
	}
	if p.Step > 0 {
		return math.Min(p.Max, p.Min+math.Round((v-p.Min)/p.Step)*p.Step)
	}
	return math.Round(v)
}

// Choice is a named set of alternatives, such as which response a biquad has.
type Choice struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Options []string `json:"options"`
	Default string   `json:"default"`
}

// Kind is one buildable filter: what it is called, what it takes, and enough
// about the ranges to drive a set of sliders.
type Kind struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Params  []Param  `json:"params"`
	Choices []Choice `json:"choices,omitempty"`
}

// Catalog returns the filters Build can make, as a fresh slice each time so no
// importer can rewrite a shared table.
//
// It exists so a front end need not know what filters there are: adding a Kind
// here adds it to the command line and the interactive page without either
// changing.
func Catalog() []Kind {
	return []Kind{
		{
			Name: "biquad", Label: "Biquad",
			Choices: []Choice{{Name: "type", Label: "Response", Default: "lowpass", Options: []string{
				"lowpass", "highpass", "bandpass-skirt", "bandpass-peak",
				"notch", "allpass", "peaking", "lowshelf", "highshelf"}}},
			Params: []Param{
				{Name: "rate", Label: "Sample rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "freq", Label: "Frequency", Min: 20, Max: 20000, Default: 1000, Step: 1, Log: true, Unit: "Hz"},
				{Name: "q", Label: "Q", Min: 0.1, Max: 20, Default: 0.7071, Step: 0.01, Log: true},
				{Name: "gain", Label: "Gain", Min: -24, Max: 24, Default: 6, Step: 0.1, Unit: "dB"},
				{Name: "slope", Label: "Shelf slope", Min: 0.1, Max: 2, Default: 1, Step: 0.05},
			},
		},
		{
			Name: "firpm", Label: "Parks-McClellan FIR",
			Params: []Param{
				{Name: "rate", Label: "Sample rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "taps", Label: "Taps", Min: 3, Max: 401, Default: 63, Step: 2, Integer: true},
				{Name: "passEnd", Label: "Passband edge", Min: 100, Max: 20000, Default: 6000, Step: 10, Unit: "Hz"},
				{Name: "stopStart", Label: "Stopband edge", Min: 200, Max: 24000, Default: 9000, Step: 10, Unit: "Hz"},
				{Name: "stopWeight", Label: "Stopband weight", Min: 0.1, Max: 100, Default: 1, Step: 0.1, Log: true},
			},
		},
		{
			Name: "firdecim", Label: "Decimating FIR",
			Params: []Param{
				{Name: "rate", Label: "Sample rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "taps", Label: "Taps", Min: 3, Max: 401, Default: 63, Step: 2, Integer: true},
				// At least 2: a factor of 1 puts the passband edge at
				// 0.8*Nyquist and the stopband edge at Nyquist itself, which
				// is a repeated band edge that firpm rejects.
				{Name: "factor", Label: "Factor", Min: 2, Max: 64, Default: 4, Step: 1, Integer: true},
			},
		},
		{
			Name: "decimate", Label: "Boxcar decimator",
			Params: []Param{
				{Name: "rate", Label: "Sample rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "factor", Label: "Factor", Min: 1, Max: 64, Default: 4, Step: 1, Integer: true},
			},
		},
		{
			Name: "rational", Label: "Rational decimator",
			Params: []Param{
				{Name: "fast", Label: "Input rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "slow", Label: "Output rate", Min: 8000, Max: 192000, Default: 44100, Step: 100, Unit: "Hz"},
			},
		},
		{
			Name: "dcblock", Label: "DC blocker",
			Params: []Param{
				{Name: "rate", Label: "Sample rate", Min: 8000, Max: 192000, Default: 48000, Step: 100, Unit: "Hz"},
				{Name: "pole", Label: "Pole", Min: 0, Max: 0.9999, Default: 0.995, Step: 0.0001},
			},
		},
	}
}

// Bands returns the passband and stopband a set of parameters implies, for the
// kinds where they are part of the specification rather than something to be
// discovered. The second return is false when the kind does not define them and
// the caller should let Analyze work them out from the response.
func Bands(kind string, params map[string]float64) (pass, stop [2]float64, ok bool) {
	if kind != "firpm" {
		return pass, stop, false
	}
	k, _ := KindByName(kind)
	pass, stop, err := firpmBands(paramGetter(k, params))
	return pass, stop, err == nil
}

// paramGetter returns a lookup that falls back to a parameter's default when it
// is absent or NaN and otherwise clamps it to the advertised range, which is
// what Build promises for every parameter.
func paramGetter(k Kind, params map[string]float64) func(string) float64 {
	return func(name string) float64 {
		v, ok := params[name]
		for _, p := range k.Params {
			if p.Name != name {
				continue
			}
			if !ok || math.IsNaN(v) {
				return p.Default
			}
			return p.snap(v)
		}
		return v
	}
}

// firpmBands works out the two bands the firpm entry designs for. Build designs
// from them and Bands reports them, so the filter a report describes is the
// filter that was measured.
//
// Both edges are clamped inside the Nyquist rate, not just the passband. The
// catalog's ranges are absolute frequencies, so dropping the sample rate can
// leave the default stopband edge above Nyquist, and the edges must stay in
// order. Clamping to 0.8 and 0.9 of Nyquist leaves a tenth of it for the
// stopband; tighter clamps design a stopband too narrow for the exchange's grid
// points, and every advertised parameter has to build at every advertised rate,
// since a slider can be moved to either end.
func firpmBands(get func(string) float64) (pass, stop [2]float64, err error) {
	nyq := get("rate") / 2
	passEnd := math.Min(get("passEnd"), nyq*0.8)
	stopStart := math.Min(get("stopStart"), nyq*0.9)
	if stopStart <= passEnd {
		stopStart = math.Min(passEnd*1.2, nyq*0.9)
	}
	if stopStart <= passEnd {
		return pass, stop, fmt.Errorf("dspviz: no room for a transition band below %g Hz", nyq)
	}
	return [2]float64{0, passEnd}, [2]float64{stopStart, nyq}, nil
}

// KindByName returns the named entry of the catalog.
func KindByName(name string) (Kind, bool) {
	for _, k := range Catalog() {
		if k.Name == name {
			return k, true
		}
	}
	return Kind{}, false
}

// Defaults returns a kind's default parameters and choices, ready to be
// overridden and handed to Build.
func (k Kind) Defaults() (params map[string]float64, choices map[string]string) {
	params = make(map[string]float64, len(k.Params))
	for _, p := range k.Params {
		params[p.Name] = p.Default
	}
	choices = make(map[string]string, len(k.Choices))
	for _, c := range k.Choices {
		choices[c.Name] = c.Default
	}
	return params, choices
}

// Build makes a Processor from a catalog entry and a set of parameters.
//
// Every parameter is clamped to the advertised range rather than rejected,
// except where a value outside it would build a different filter than asked. A
// slider caller should not have to validate what the slider bounded, and a
// network caller must not be able to ask for something unbounded.
func Build(kind string, params map[string]float64, choices map[string]string) (Processor, error) {
	k, ok := KindByName(kind)
	if !ok {
		return nil, fmt.Errorf("dspviz: unknown filter kind %q", kind)
	}
	get := paramGetter(k, params)
	pick := func(name string) string {
		v := choices[name]
		for _, c := range k.Choices {
			if c.Name != name {
				continue
			}
			if slices.Contains(c.Options, v) {
				return v
			}
			return c.Default
		}
		return v
	}

	switch kind {
	case "biquad":
		rate, freq, q := get("rate"), get("freq"), get("q")
		freq = math.Min(freq, rate/2*0.999) // above Nyquist the cookbook formulas are meaningless
		gain, slope := get("gain"), get("slope")
		var (
			f   *dsp.BiQuadFilter[float64]
			err error
		)
		switch pick("type") {
		case "lowpass":
			f, err = dsp.NewLowPassBiQuad[float64](rate, freq, q)
		case "highpass":
			f, err = dsp.NewHighPassBiQuad[float64](rate, freq, q)
		case "bandpass-skirt":
			f, err = dsp.NewBandPassConstantSkirtGainBiQuad[float64](rate, freq, q)
		case "bandpass-peak":
			f, err = dsp.NewBandPassConstantPeakGainBiQuad[float64](rate, freq, q)
		case "notch":
			f, err = dsp.NewNotchBiQuad[float64](rate, freq, q)
		case "allpass":
			f, err = dsp.NewAllPassBiQuad[float64](rate, freq, q)
		case "peaking":
			f, err = dsp.NewPeakingEQBiQuad[float64](rate, freq, q, gain)
		case "lowshelf":
			f, err = dsp.NewLowShelfBiQuad[float64](rate, freq, slope, gain)
		case "highshelf":
			f, err = dsp.NewHighShelfBiQuad[float64](rate, freq, slope, gain)
		default:
			// pick falls back to the choice's default, so this is reachable
			// only if the catalog lists an option this switch does not build.
			return nil, fmt.Errorf("dspviz: biquad type %q is in the catalog but not built here", pick("type"))
		}
		if err != nil {
			return nil, err
		}
		return NewBiQuadProcessor(f, rate)

	case "firpm":
		rate := get("rate")
		pass, stop, err := firpmBands(get)
		if err != nil {
			return nil, err
		}
		h, _, err := firpm.Design(firpm.Spec{
			NumTaps: int(math.Round(get("taps"))), SampleRate: rate,
			Bands: []firpm.Band{
				{Lower: pass[0], Upper: pass[1], Response: 1, Weight: 1},
				{Lower: stop[0], Upper: stop[1], Response: 0, Weight: get("stopWeight")},
			},
		})
		if err != nil && h == nil {
			return nil, fmt.Errorf("dspviz: %w", err)
		}
		return NewFIRProcessor(h, rate)

	case "firdecim":
		// The passband is the output Nyquist, which is what the decimator has
		// to protect: everything above it folds back on the way down.
		rate := get("rate")
		factor := int(math.Round(get("factor")))
		outNyq := rate / float64(factor) / 2
		h, _, err := firpm.Design(firpm.Spec{
			NumTaps: int(math.Round(get("taps"))), SampleRate: rate,
			Bands: []firpm.Band{
				{Lower: 0, Upper: outNyq * 0.8, Response: 1, Weight: 1},
				{Lower: outNyq, Upper: rate / 2, Response: 0, Weight: 1},
			},
		})
		if err != nil && h == nil {
			return nil, fmt.Errorf("dspviz: %w", err)
		}
		return NewFIRDecimatorProcessor(h, factor, rate)

	case "decimate":
		f, err := dsp.NewBoxcarDecimator(int(math.Round(get("factor"))))
		if err != nil {
			return nil, err
		}
		return NewComplexDownsampleProcessor(f, get("rate"))

	case "rational":
		fast, slow := int(math.Round(get("fast"))), int(math.Round(get("slow")))
		if slow > fast {
			slow = fast
		}
		f, err := dsp.NewRationalBoxcarDecimator(fast, slow)
		if err != nil {
			return nil, err
		}
		return NewRationalDownsampleProcessor(f)

	case "dcblock":
		f, err := dsp.NewDCFilter[float64](get("pole"))
		if err != nil {
			return nil, err
		}
		return NewDCProcessor(f, get("rate"))
	}
	return nil, fmt.Errorf("dspviz: filter kind %q is in the catalog but Build does not make it", kind)
}
