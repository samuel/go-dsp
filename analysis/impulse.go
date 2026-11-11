package analysis

import "samuelks.com/dsp"

// Impulse returns the response of p to a unit impulse followed by zeros,
// starting from a reset state, over n input samples.
//
// The result is at the output rate, so a decimator by four fed n samples
// returns about n/4 of them.
func Impulse(p dsp.Processor[float64, float64], n int) []float64 {
	return ImpulseAt(p, n, 0)
}

// ImpulseAt is Impulse with the impulse placed phase samples in.
//
// A rate-changing processor is periodically time-varying, so it has no single
// impulse response: a decimator by four has four, one per position the impulse
// can take relative to the output clock, and phase picks among them. The
// published converter tests get them all with a train of impulses, which they
// need because they drive the converter as an external program; here the
// processor can just be reset and re-run. See PolyphaseImpulse.
func ImpulseAt(p dsp.Processor[float64, float64], n, phase int) []float64 {
	if n <= 0 {
		return nil
	}
	if phase < 0 {
		phase = 0
	}
	in := make([]float64, n)
	if phase < n {
		in[phase] = 1
	}
	return run(p, nil, in)
}

// PolyphaseImpulse returns one impulse response per phase of a rate-changing
// processor, over n input samples each. A processor whose rates match has a
// single phase, so the result holds one response.
//
// The number of phases is the input count of p's Ratio divided by the greatest
// common divisor of the two counts: three of every four input samples land between outputs for
// a decimator by four, and each of those positions is a different filter.
func PolyphaseImpulse(p dsp.Processor[float64, float64], n int) [][]float64 {
	phases := Phases(p)
	out := make([][]float64, phases)
	for i := range out {
		out[i] = ImpulseAt(p, n, i)
	}
	return out
}

// Phases returns how many input samples pass before a rate-changing processor
// returns to the same position relative to its output clock. It is 1 for a
// processor without a Ratio method, which does not change the rate.
func Phases(p dsp.Processor[float64, float64]) int {
	r, ok := p.(Ratio)
	if !ok {
		return 1
	}
	in, out := r.Ratio()
	if in <= 0 || out <= 0 {
		return 1
	}
	return in / gcd(in, out)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Step returns the response of p to n samples of unity, from a reset state.
func Step(p dsp.Processor[float64, float64], n int) []float64 {
	if n <= 0 {
		return nil
	}
	in := make([]float64, n)
	for i := range in {
		in[i] = 1
	}
	return run(p, nil, in)
}
