package dspviz

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"strconv"

	"github.com/samuel/go-dsp/dsp"
)

// Method selects how a frequency response is arrived at.
type Method int

const (
	// MethodAuto takes MethodExact when the Processor implements Coefficients
	// and MethodTone otherwise.
	MethodAuto Method = iota

	// MethodExact evaluates the transfer function with dsp.Freqz. It has no
	// truncation, transient, window or grid of its own, so it is the only method
	// that resolves a hundredth of a decibel of ripple, and the plot is free to
	// space the frequencies. It needs coefficients, so it cannot measure a rate
	// changer or anything nonlinear.
	MethodExact

	// MethodTone injects one coherent tone per frequency and reads the
	// corresponding component out of the output. It is exact for a linear
	// time-invariant filter up to the residue of the startup transient, and it
	// is the only method that stays meaningful for a rate changer.
	MethodTone

	// MethodImpulse transforms a truncated impulse response. It is a fast
	// survey rather than a measurement: the discarded tail puts a floor under
	// the result, and for an IIR filter that floor lands exactly where the
	// stopband gets interesting.
	MethodImpulse
)

func (m Method) String() string {
	switch m {
	case MethodAuto:
		return "auto"
	case MethodExact:
		return "exact"
	case MethodTone:
		return "tone"
	case MethodImpulse:
		return "impulse"
	}
	return "method(" + strconv.Itoa(int(m)) + ")"
}

// ResponseOptions tunes how FreqResponse measures. The zero value is the
// sensible default for every field.
type ResponseOptions struct {
	// Method selects the measurement. Zero is MethodAuto.
	Method Method

	// Frame is the number of input samples the response is read from, for the
	// measured methods. Zero selects 65536. MethodImpulse rounds it up to a
	// power of two; MethodTone does not need one.
	Frame int

	// Settle is how many samples are discarded before the frame, so the startup
	// transient does not land in it. Zero computes one from the filter's poles
	// where the coefficients allow, and otherwise takes eight frames.
	Settle int

	// Amplitude of the injected tone. Zero selects 1.
	Amplitude float64

	// Normalize divides the whole response by its gain at the lowest frequency
	// measured, so a filter with a gain reads 0 dB in its passband.
	Normalize bool
}

// Response is a magnitude and phase measurement over a frequency grid.
//
// Freq is in Hz against the input rate; H is the complex response at each. For
// a rate-changing processor H holds the ratio of the wanted output component to
// the input tone, and says nothing about what the rate change added alongside
// it.
type Response struct {
	Rate   float64 // the input sample rate
	Freq   []float64
	H      []complex128
	Method Method

	// Warnings records anything that limits the result: a measurement that
	// could not be made coherently, a grid too coarse for the phase to unwrap.
	Warnings []string

	// b and a are the transfer function MethodExact evaluated, kept so that
	// GroupDelay can be the closed form rather than a difference of the
	// unwrapped phase. They are nil for a measured response, which has no
	// coefficients to work from.
	b, a []float64
}

// FreqResponse measures the response of p at each frequency in freq, which are
// in Hz against p's input rate.
func FreqResponse(p Processor, freq []float64, opt ResponseOptions) (*Response, error) {
	if p == nil {
		return nil, errors.New("dspviz: nil processor")
	}
	if len(freq) == 0 {
		return nil, errors.New("dspviz: no frequencies to measure")
	}
	inRate, outRate := p.Rates()

	method := opt.Method
	coef, hasCoef := p.(Coefficients)
	if method == MethodAuto {
		if hasCoef {
			method = MethodExact
		} else {
			method = MethodTone
		}
	}
	if method == MethodExact && !hasCoef {
		return nil, errors.New("dspviz: MethodExact needs a processor with coefficients")
	}

	r := &Response{Rate: inRate, Freq: append([]float64(nil), freq...), Method: method}

	var err error
	switch method {
	case MethodExact:
		r.b, r.a = coef.Coefficients()
		w := make([]float64, len(freq))
		for i, f := range freq {
			w[i] = 2 * math.Pi * f / inRate
		}
		r.H = dsp.Freqz(r.b, r.a, w)
	case MethodImpulse:
		err = impulseResponse(r, p, opt)
	case MethodTone:
		err = toneResponse(r, p, opt, inRate, outRate)
	default:
		err = fmt.Errorf("dspviz: unknown method %v", method)
	}
	if err != nil {
		return nil, err
	}

	if opt.Normalize {
		if g := cmplx.Abs(r.H[0]); g > 0 {
			for i := range r.H {
				r.H[i] /= complex(g, 0)
			}
		}
	}
	return r, nil
}

// impulseResponse transforms a truncated impulse response. It evaluates that
// response as an FIR filter rather than transforming it, which gives the same
// numbers at the transform's own bins while allowing any grid.
func impulseResponse(r *Response, p Processor, opt ResponseOptions) error {
	n := opt.Frame
	if n <= 0 {
		n = defaultFrame
	}
	h := Impulse(p, n)
	if len(h) == 0 {
		return errors.New("dspviz: the processor produced no output for an impulse")
	}
	w := make([]float64, len(r.Freq))
	_, outRate := p.Rates()
	for i, f := range r.Freq {
		w[i] = 2 * math.Pi * f / outRate
	}
	r.H = dsp.Freqz(h, nil, w)
	r.Warnings = append(r.Warnings, fmt.Sprintf(
		"impulse response truncated to %d samples; anything below the energy in the discarded tail is not real", n))
	return nil
}

const defaultFrame = 1 << 16

// toneResponse injects one coherent tone per frequency.
func toneResponse(r *Response, p Processor, opt ResponseOptions, inRate, outRate float64) error {
	frame := opt.Frame
	if frame <= 0 {
		frame = defaultFrame
	}
	amp := opt.Amplitude
	if amp == 0 {
		amp = 1
	}

	settle := opt.Settle
	if settle <= 0 {
		settle = settleFor(p, frame)
	}
	// Round the settling time to a whole number of frames so the tone's phase at
	// the start of the analysis window is the same as at sample zero, against
	// which the correlation below measures. Any other length offsets every
	// measured phase by a constant and rotates the response.
	settle = (settle + frame - 1) / frame * frame

	// Coherence needs the output frame to hold a whole number of cycles. That is
	// automatic when the rates match and often holds when they do not -- a
	// decimator by four fed a multiple of four returns exactly a quarter as many
	// samples -- so test for it rather than assume from the ratio. Where it
	// fails, a window has to bound the leakage instead.
	in := make([]float64, settle+frame)
	var out, buf, win []float64
	r.H = make([]complex128, len(r.Freq))
	windowed := false

	for i, f := range r.Freq {
		hz, k := CoherentFreq(f, inRate, frame)
		r.Freq[i] = hz
		Tone(in, k, frame, amp)

		p.Reset()
		out = p.Process(out[:0], in)

		// Take the tail; the settle samples at the front are thrown away.
		want := int(math.Round(float64(frame) * outRate / inRate))
		if want <= 0 || want > len(out) {
			return fmt.Errorf("dspviz: the processor returned %d samples, too few for a frame of %d", len(out), want)
		}
		tail := out[len(out)-want:]

		cycles := hz * float64(len(tail)) / outRate
		if n := math.Round(cycles); math.Abs(cycles-n) < 1e-9 {
			r.H[i] = dsp.DFTBinReal(tail, int(n)) / complex(amp, 0)
			continue
		}

		// Off-bin at the output, which is what a fractional rate change leaves.
		// Window, and divide out the window's coherent gain so the amplitude
		// still reads true.
		windowed = true
		win = resize(win, len(tail))
		dsp.KaiserWindow(win, analysisBeta)
		buf = resize(buf, len(tail))
		for j, v := range tail {
			buf[j] = v * win[j]
		}
		gain, _ := dsp.WindowGain(win)
		r.H[i] = offBinAmplitude(buf, cycles) * complex(float64(len(tail))/gain, 0) / complex(amp, 0)
	}

	if windowed {
		r.Warnings = append(r.Warnings, fmt.Sprintf(
			"the rate change from %g to %g leaves the tone off-bin at the output, so it cannot be read back "+
				"coherently; a Kaiser window bounds the leakage near %.0f dB", inRate, outRate, analysisSidelobeDB))
	}
	return nil
}

// analysisBeta is the Kaiser shape used wherever a measurement cannot be made
// coherently. Do not derive it from dsp.KaiserBeta: that is the FIR design
// formula, whose argument is a filter's attenuation and not a sidelobe level,
// and it gives a much noisier window for the same number.
const (
	analysisBeta       = 19.4
	analysisSidelobeDB = -150.0
)

// offBinAmplitude is dsp.DFTBinReal for a cycle count that is not a whole
// number, which is what a rate change leaves. It cannot be exact, which is why
// the caller windows first.
func offBinAmplitude(x []float64, cycles float64) complex128 {
	n := len(x)
	if n == 0 {
		return 0
	}
	var re, im float64
	for i, v := range x {
		s, c := math.Sincos(2 * math.Pi * cycles * float64(i) / float64(n))
		re += v * c
		im -= v * s
	}
	return 2 * complex(re, im) / complex(float64(n), 0)
}

// settleFor returns how many samples to discard before measuring, from the
// filter's slowest pole where that can be worked out and from a generous
// multiple of the frame where it cannot.
func settleFor(p Processor, frame int) int {
	coef, ok := p.(Coefficients)
	if !ok {
		return 8 * frame
	}
	b, a := coef.Coefficients()
	r, ok := poleRadius(a)
	if !ok || r <= 0 {
		// No feedback, so the transient is over once the delay line has filled.
		return len(b) + 1
	}
	if r >= 1 {
		return 8 * frame // unstable or marginal; nothing will settle
	}
	// The envelope decays as r^n, so falling 300 dB takes this many samples.
	n := int(math.Ceil(300 / 20 * math.Ln10 / math.Log(1/r)))
	return min(max(n, 64), 1<<22)
}

// poleRadius returns the magnitude of the largest root of the denominator, for
// the orders where that has a closed form. Higher orders report false and the
// caller falls back on a generous fixed settling time.
func poleRadius(a []float64) (float64, bool) {
	switch len(a) {
	case 0, 1:
		return 0, true // no feedback at all
	case 2:
		if a[0] == 0 {
			return 0, false
		}
		return math.Abs(a[1] / a[0]), true
	case 3:
		if a[0] == 0 {
			return 0, false
		}
		b, c := a[1]/a[0], a[2]/a[0]
		disc := b*b - 4*c
		if disc < 0 {
			// Complex conjugate pair, both of magnitude sqrt(c).
			return math.Sqrt(math.Abs(c)), true
		}
		s := math.Sqrt(disc)
		return math.Max(math.Abs((-b+s)/2), math.Abs((-b-s)/2)), true
	}
	return 0, false
}

// Magnitude returns |H| at each frequency, as a fresh slice.
func (r *Response) Magnitude() []float64 {
	m := make([]float64, len(r.H))
	for i, v := range r.H {
		m[i] = cmplx.Abs(v)
	}
	return m
}

// MagnitudeDB returns 20*log10|H| at each frequency. A true zero comes back as
// negative infinity, which every plot here clips at its own floor rather than
// dropping.
func (r *Response) MagnitudeDB() []float64 {
	m := make([]float64, len(r.H))
	for i, v := range r.H {
		m[i] = dbAmplitude(cmplx.Abs(v))
	}
	return m
}

// Phase returns the phase of H in radians, wrapped into (-pi, pi].
func (r *Response) Phase() []float64 {
	p := make([]float64, len(r.H))
	for i, v := range r.H {
		p[i] = math.Atan2(imag(v), real(v))
	}
	return p
}

// UnwrappedPhase returns the phase with the 2*pi jumps taken out, so a
// steadily falling curve reads as a line rather than a sawtooth.
//
// Unwrapping assumes the true phase moves by less than pi between neighbors, a
// statement about the grid: a filter delaying by d samples needs a step below
// pi/d radians per sample. GridWarning reports when the grid is too coarse.
func (r *Response) UnwrappedPhase() []float64 {
	p := r.Phase()
	dsp.Unwrap(p)
	return p
}

// ResidualPhase returns the unwrapped phase with the best-fit straight line
// removed, in degrees. Pure delay is a straight line in phase and says nothing
// about a filter beyond its latency, so removing it makes a linear-phase filter
// read as flat and leaves real phase distortion visible against it.
//
// Where the magnitude is negligible the phase is meaningless, and for a
// linear-phase FIR it is worse: the amplitude changes sign at every stopband
// null, a genuine jump of pi that no unwrapping can tell from a wrap. Those
// points are excluded from the fit, or the stopband drags the line away and
// slopes the passband. They are still returned; PhaseChart drops them from the
// picture.
func (r *Response) ResidualPhase() []float64 {
	p := r.UnwrappedPhase()
	n := len(p)
	if n < 2 {
		return make([]float64, n)
	}
	use := r.significant()

	// Least squares fit of phase against frequency, over the points that carry
	// enough signal to mean anything.
	var sx, sy, sxx, sxy, fn float64
	for i, v := range p {
		if !use[i] {
			continue
		}
		x := r.Freq[i]
		sx += x
		sy += v
		sxx += x * x
		sxy += x * v
		fn++
	}
	if fn < 2 {
		sx, sy, sxx, sxy, fn = 0, 0, 0, 0, 0
		for i, v := range p {
			x := r.Freq[i]
			sx += x
			sy += v
			sxx += x * x
			sxy += x * v
			fn++
		}
	}
	den := fn*sxx - sx*sx
	var slope, intercept float64
	if den != 0 {
		slope = (fn*sxy - sx*sy) / den
		intercept = (sy - slope*sx) / fn
	} else {
		intercept = sy / fn
	}
	out := make([]float64, n)
	for i, v := range p {
		out[i] = (v - (slope*r.Freq[i] + intercept)) * 180 / math.Pi
	}
	return out
}

// phaseFloorDB is how far below the peak a magnitude has to fall before its
// phase is treated as meaningless.
const phaseFloorDB = -80.0

// significant reports, per frequency, whether the magnitude is far enough above
// nothing for the phase there to mean something.
func (r *Response) significant() []bool {
	use := make([]bool, len(r.H))
	var peak float64
	for _, v := range r.H {
		peak = math.Max(peak, cmplx.Abs(v))
	}
	if peak == 0 {
		return use
	}
	cutoff := peak * math.Pow(10, phaseFloorDB/20)
	for i, v := range r.H {
		use[i] = cmplx.Abs(v) >= cutoff
	}
	return use
}

// GroupDelay returns the group delay in samples at the input rate.
//
// An exactly measured response uses the closed form, which does not depend on
// grid spacing. A measured response central-differences the unwrapped phase and
// is only as good as the grid; the Response says so in its warnings.
//
// That distinction is what lets GridWarning say anything: a differenced delay
// comes off a phase that never steps past pi by construction, so it can never
// find the grid wanting. The closed form has no such ceiling.
func (r *Response) GroupDelay() []float64 {
	n := len(r.H)
	d := make([]float64, n)
	if n == 0 {
		return d
	}
	w := make([]float64, n)
	for i, f := range r.Freq {
		w[i] = 2 * math.Pi * f / r.Rate
	}
	if r.b != nil {
		return dsp.GroupDelay(r.b, r.a, w)
	}
	if n < 2 {
		return d
	}
	p := r.UnwrappedPhase()
	for i := range d {
		lo, hi := max(i-1, 0), min(i+1, n-1)
		if dw := w[hi] - w[lo]; dw != 0 {
			d[i] = -(p[hi] - p[lo]) / dw
		}
	}
	return d
}

// GroupDelaySeconds is GroupDelay in seconds rather than samples.
func (r *Response) GroupDelaySeconds() []float64 {
	d := r.GroupDelay()
	for i := range d {
		d[i] /= r.Rate
	}
	return d
}

// GridWarning warns when the grid is too coarse for the phase to unwrap: the
// phase moves by more than pi between neighbors.
//
// Only an exactly measured response can answer this; see GroupDelay for why. A
// measured response therefore never warns, which is honest since it never has
// the information the test needs -- the coarse grid threw it away.
func (r *Response) GridWarning() string {
	d := r.GroupDelay()
	var worst float64
	for _, v := range d {
		worst = math.Max(worst, math.Abs(v))
	}
	if worst == 0 || len(r.Freq) < 2 {
		return ""
	}
	step := math.Abs(r.Freq[1]-r.Freq[0]) * 2 * math.Pi / r.Rate
	if step*worst > math.Pi {
		return fmt.Sprintf("the grid steps %.3g radians per sample against a group delay of %.0f samples, "+
			"which is too coarse to unwrap the phase reliably", step, worst)
	}
	return ""
}
