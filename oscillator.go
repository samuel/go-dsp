package dsp

import (
	"errors"
	"math"
)

// phasor is the phase accumulator an Oscillator and a Mixer share.
//
// The phase is in cycles on [0, 1) rather than radians, so wrapping is an exact
// subtraction of 1 and a frequency that is a binary fraction, such as 0.25,
// steps without error. Each step rounds once, by at most 2^-53 cycles, so after
// n samples the phase is within n*2^-53 cycles of exact.
type phasor struct {
	freq  float64 // as given, for Frequency
	step  float64 // freq reduced to [0, 1)
	phase float64
	init  float64 // the constructor's frequency, which Reset restores
}

func newPhasor(freq float64) (phasor, error) {
	var p phasor
	if err := p.setFrequency(freq); err != nil {
		return p, err
	}
	p.init = freq
	return p, nil
}

func (p *phasor) setFrequency(freq float64) error {
	if math.IsNaN(freq) || math.IsInf(freq, 0) {
		return errors.New("dsp: the frequency must be finite")
	}
	step := freq - math.Floor(freq)
	if step >= 1 {
		// A tiny negative frequency, whose reduction rounds up to a whole cycle.
		step = 0
	}
	p.freq, p.step = freq, step
	return nil
}

// next returns the sine and cosine of the current phase and advances it.
func (p *phasor) next() (sin, cos float64) {
	sin, cos = math.Sincos(2 * math.Pi * p.phase)
	p.phase += p.step
	if p.phase >= 1 {
		p.phase--
	}
	return sin, cos
}

func (p *phasor) reset() {
	p.phase = 0
	_ = p.setFrequency(p.init) // init was accepted by the constructor
}

// Oscillator generates a phase-continuous tone: cos(2*pi*f*n) for a real T and
// exp(+j*2*pi*f*n) for a complex one, with f in cycles per sample. It carries
// its phase across calls, so a tone generated in blocks of any length is one
// tone, and SetFrequency changes the frequency without a phase jump.
//
// Every sample is one math.Sincos in float64, narrowed once to T. Not safe for
// concurrent use.
type Oscillator[T Sample] struct {
	p phasor
}

// NewOscillator returns an oscillator at phase zero. freq is in cycles per
// sample; a negative one turns a complex tone the other way, and one outside
// [-0.5, 0.5] aliases as a sampled tone does. It must be finite.
func NewOscillator[T Sample](freq float64) (*Oscillator[T], error) {
	p, err := newPhasor(freq)
	if err != nil {
		return nil, err
	}
	return &Oscillator[T]{p: p}, nil
}

// Frequency returns the frequency in cycles per sample, as it was last set.
func (o *Oscillator[T]) Frequency() float64 { return o.p.freq }

// SetFrequency changes the frequency from the next sample on, continuing from
// the current phase.
func (o *Oscillator[T]) SetFrequency(freq float64) error { return o.p.setFrequency(freq) }

// Reset returns the oscillator to phase zero at the frequency it was
// constructed with.
func (o *Oscillator[T]) Reset() { o.p.reset() }

// Generate fills dst with the next len(dst) samples of the tone.
func (o *Oscillator[T]) Generate(dst []T) {
	switch d := any(dst).(type) {
	case []float32:
		generateReal(&o.p, d)
	case []float64:
		generateReal(&o.p, d)
	case []complex64:
		generateComplex(&o.p, d)
	case []complex128:
		generateComplex(&o.p, d)
	default:
		panic("dsp: Oscillator.Generate: unsupported sample type")
	}
}

func generateReal[F Float](p *phasor, dst []F) {
	for i := range dst {
		_, c := p.next()
		dst[i] = F(c)
	}
}

func generateComplex[C Complex](p *phasor, dst []C) {
	for i := range dst {
		s, c := p.next()
		dst[i] = C(complex(c, s))
	}
}

// Mixer multiplies a stream by exp(+j*2*pi*f*n), f in cycles per sample, so a
// positive frequency shifts the spectrum up and a negative one down. A real In
// takes a real IF to complex baseband, and with In the same as Out the mix can
// run in place. The local oscillator's phase carries across calls.
//
// The product is formed in complex128 and narrowed once to Out. Not safe for
// concurrent use.
type Mixer[In Sample, Out Complex] struct {
	p phasor
}

// NewMixer returns a mixer whose oscillator starts at phase zero. freq must be
// finite.
func NewMixer[In Sample, Out Complex](freq float64) (*Mixer[In, Out], error) {
	p, err := newPhasor(freq)
	if err != nil {
		return nil, err
	}
	return &Mixer[In, Out]{p: p}, nil
}

// Frequency returns the shift in cycles per sample, as it was last set.
func (m *Mixer[In, Out]) Frequency() float64 { return m.p.freq }

// SetFrequency changes the shift from the next sample on, continuing from the
// oscillator's current phase.
func (m *Mixer[In, Out]) SetFrequency(freq float64) error { return m.p.setFrequency(freq) }

// OutputLen returns inputs: the mixer writes one sample per input.
func (m *Mixer[In, Out]) OutputLen(inputs int) int { return inputs }

// Reset returns the oscillator to phase zero at the frequency the mixer was
// constructed with.
func (m *Mixer[In, Out]) Reset() { m.p.reset() }

// Process writes src shifted by the mixer's frequency to dst and returns
// len(src). It panics if dst is shorter than src. With In the same as Out, dst
// may be src.
func (m *Mixer[In, Out]) Process(dst []Out, src []In) int {
	checkDst("Mixer", len(dst), len(src))
	dst = dst[:len(src)]
	switch s := any(src).(type) {
	case []float32:
		mixReal(&m.p, dst, s)
	case []float64:
		mixReal(&m.p, dst, s)
	case []complex64:
		mixComplex(&m.p, dst, s)
	case []complex128:
		mixComplex(&m.p, dst, s)
	default:
		panic("dsp: Mixer.Process: unsupported sample type")
	}
	return len(src)
}

func mixReal[F Float, C Complex](p *phasor, dst []C, src []F) {
	for i, x := range src {
		s, c := p.next()
		v := float64(x)
		dst[i] = C(complex(v*c, v*s))
	}
}

func mixComplex[CI, CO Complex](p *phasor, dst []CO, src []CI) {
	for i, x := range src {
		s, c := p.next()
		dst[i] = CO(complex128(x) * complex(c, s))
	}
}
