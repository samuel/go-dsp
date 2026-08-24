package dspviz

// Impulse returns the response of p to a unit impulse followed by zeros,
// starting from a reset state, over n input samples.
//
// The result is at the output rate, so a decimator by four fed n samples
// returns about n/4 of them.
func Impulse(p Processor, n int) []float64 {
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
func ImpulseAt(p Processor, n, phase int) []float64 {
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
	p.Reset()
	return p.Process(nil, in)
}

// PolyphaseImpulse returns one impulse response per phase of a rate-changing
// processor, over n input samples each. A processor whose rates match has a
// single phase, so the result holds one response.
//
// The number of phases is the input rate divided by the greatest common divisor
// of the two rates: three of every four input samples land between outputs for
// a decimator by four, and each of those positions is a different filter.
func PolyphaseImpulse(p Processor, n int) [][]float64 {
	phases := Phases(p)
	out := make([][]float64, phases)
	for i := range out {
		out[i] = ImpulseAt(p, n, i)
	}
	return out
}

// Phases returns how many input samples pass before a rate-changing processor
// returns to the same position relative to its output clock. It is 1 for a
// processor that does not change the rate.
func Phases(p Processor) int {
	in, out := p.Rates()
	if in == out {
		return 1
	}
	a, _ := ratio(in, out)
	if a <= 0 {
		return 1
	}
	return a
}

// ratio expresses in/out as a fraction in the smallest whole numbers it can,
// for rates that are whole numbers or simple fractions of one. It gives up and
// reports zero for anything else, since a period that is not a whole number of
// samples is not a period.
func ratio(in, out float64) (num, den int) {
	const limit = 1 << 20
	ni, no := int(in), int(out)
	if float64(ni) != in || float64(no) != out || ni <= 0 || no <= 0 {
		return 0, 0
	}
	g := gcd(ni, no)
	num, den = ni/g, no/g
	if num > limit {
		return 0, 0
	}
	return num, den
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Step returns the response of p to n samples of unity, from a reset state.
func Step(p Processor, n int) []float64 {
	if n <= 0 {
		return nil
	}
	in := make([]float64, n)
	for i := range in {
		in[i] = 1
	}
	p.Reset()
	return p.Process(nil, in)
}
