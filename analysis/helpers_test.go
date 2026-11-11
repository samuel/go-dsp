package analysis

import "samuelks.com/dsp"

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// funcProcessor is a dsp.Processor[float64, float64] over a function that
// appends one output per input. reset may be nil.
type funcProcessor struct {
	fn    func(dst, src []float64) []float64
	reset func()
}

// processorFunc adapts fn, for a device the dsp types do not cover.
func processorFunc(fn func(dst, src []float64) []float64, reset func()) dsp.Processor[float64, float64] {
	return &funcProcessor{fn: fn, reset: reset}
}

// identity passes its input through unchanged. It is the reference the
// measurement is checked against: anything it reports other than a flat unity
// response is the measurement's own error rather than a filter's.
func identity() dsp.Processor[float64, float64] {
	return processorFunc(func(dst, src []float64) []float64 { return append(dst, src...) }, nil)
}

func (p *funcProcessor) OutputLen(inputs int) int { return inputs }

func (p *funcProcessor) Process(dst, src []float64) int {
	out := p.fn(dst[:0], src)
	if len(out) != len(src) {
		panic("funcProcessor: the function must write one output per input")
	}
	copy(dst, out)
	return len(out)
}

func (p *funcProcessor) Reset() {
	if p.reset != nil {
		p.reset()
	}
}
