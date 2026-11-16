package dsp

import (
	"errors"
	"math"
)

// pllDamping is the loop's damping factor, critically damped enough to settle without
// ringing.
const pllDamping = math.Sqrt2 / 2

// PLL tracks the carrier of a complex signal with a second-order phase-locked loop and writes
// the signal derotated by it: the carrier lands on the positive real axis, so for AM the real
// part is the carrier plus the modulation. The loop's phase detector is the angle of the
// derotated sample, so it locks to the strongest component near its frequency. Its state is
// float64 at every width. Not safe for concurrent use.
type PLL[C Complex] struct {
	alpha, beta float64 // proportional and integral gains
	maxFreq     float64 // radians per sample
	phase, freq float64 // radians, radians per sample
}

// NewPLL returns a loop with the given noise bandwidth, in cycles per sample, that tracks a
// carrier within maxFreq cycles per sample of 0 Hz. A wider loop locks faster and follows
// drift better; a narrower one rejects more noise and modulation. The loop starts at 0 Hz.
func NewPLL[C Complex](loopBandwidth, maxFreq float64) (*PLL[C], error) {
	switch {
	case !(loopBandwidth > 0 && loopBandwidth < 0.5):
		return nil, errors.New("dsp: PLL loop bandwidth must be in (0, 0.5) cycles per sample")
	case !(maxFreq >= 0 && maxFreq <= 0.5):
		return nil, errors.New("dsp: PLL frequency range must be in [0, 0.5] cycles per sample")
	}
	// Gains of a second-order loop with a proportional-plus-integral filter, from its noise
	// bandwidth and damping (Rice, Digital Communications, appendix C).
	theta := 2 * math.Pi * loopBandwidth / (pllDamping + 1/(4*pllDamping))
	d := 1 + 2*pllDamping*theta + theta*theta
	return &PLL[C]{
		alpha:   4 * pllDamping * theta / d,
		beta:    4 * theta * theta / d,
		maxFreq: 2 * math.Pi * maxFreq,
	}, nil
}

// Frequency returns the tracked carrier frequency in cycles per sample.
func (p *PLL[C]) Frequency() float64 { return p.freq / (2 * math.Pi) }

// OutputLen returns inputs: the loop writes one sample per input.
func (p *PLL[C]) OutputLen(inputs int) int { return inputs }

// Process writes src derotated by the tracked carrier to dst and returns len(src). It panics
// if dst is shorter than src. dst may be src.
func (p *PLL[C]) Process(dst, src []C) int {
	checkDst("PLL", len(dst), len(src))
	for i, x := range src {
		s, c := math.Sincos(p.phase)
		v := complex128(x) * complex(c, -s)
		dst[i] = C(v)
		err := math.Atan2(imag(v), real(v))
		p.freq = max(-p.maxFreq, min(p.maxFreq, p.freq+p.beta*err))
		p.phase = math.Remainder(p.phase+p.freq+p.alpha*err, 2*math.Pi)
	}
	return len(src)
}

// Reset returns the loop to 0 Hz and phase zero.
func (p *PLL[C]) Reset() { p.phase, p.freq = 0, 0 }
