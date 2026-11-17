package dsp

import (
	"errors"
	"math"
	"math/cmplx"
)

// TimingDetector is the timing error detector a SymbolSync steers its clock with.
type TimingDetector int

const (
	// Gardner compares the sample midway between two symbols with their difference. It needs
	// no decisions, so it works before carrier recovery, but it sees only symbols that change.
	Gardner TimingDetector = iota
	// MuellerMuller correlates each symbol with the decision on its neighbor. It uses only the
	// symbols, not the samples between them, and suits pulses with zero crossings at the
	// symbol spacing, such as raised cosine, but needs the carrier already removed.
	MuellerMuller
)

// SymbolSyncConfig sets up a SymbolSync.
type SymbolSyncConfig struct {
	// SamplesPerSymbol is the nominal input samples per symbol, at least 2 for either
	// detector. It need not be a whole number.
	SamplesPerSymbol float64
	// LoopBandwidth is the timing loop's noise bandwidth in cycles per symbol, in (0, 0.5).
	// About 0.01 to 0.05 suits most signals: wider locks faster, narrower jitters less.
	LoopBandwidth float64
	// MaxDeviation is the largest difference between the symbol rate and the nominal one that
	// the loop follows, as a fraction of the nominal rate, in [0, 0.5).
	MaxDeviation float64
	// Detector is the timing error detector.
	Detector TimingDetector
}

// symSyncPowerAlpha is the weight, per symbol, of the running power estimate that makes the
// timing error independent of the signal's level.
const symSyncPowerAlpha = 1.0 / 32

// symSyncDetectorGain is the timing error detectors' approximate gain, error per symbol of
// timing offset, at unit power, which the loop gains are divided by.
const symSyncDetectorGain = 1.5

// SymbolSync recovers the symbol clock of a pulse-shaped stream and writes one sample per
// symbol, interpolated at the symbol's center with a cubic (four-point Lagrange) Farrow
// interpolator. A second-order loop, with PLL's damping, steers the clock by a timing error
// detector's output normalized by a running estimate of the symbol power, so the input's level
// does not change the loop's bandwidth.
//
// The output count depends on the recovered clock, so OutputLen is an upper bound and Process
// returns the count it wrote. The output does not depend on how the input is split into
// blocks. Its state is float64 at every width. Not safe for concurrent use.
type SymbolSync[T Sample] struct {
	sps         float64
	alpha, beta float64
	maxDev      float64
	detector    TimingDetector
	minStep     float64 // the shortest step between symbols, which bounds OutputLen

	// hist holds the input still needed: from one sample before the previous symbol. The next
	// symbol is at index idx plus frac in it and the previous at prevIdx plus prevFrac. Each
	// fraction is in [0, 1), so its rounding does not depend on the position in hist.
	hist           []T
	idx, prevIdx   int
	frac, prevFrac float64
	started        bool       // whether the previous position and last hold a symbol
	last           complex128 // the previous symbol
	power          float64    // running estimate of the symbol power
	integ          float64    // the loop's integrator: the rate offset, as a fraction of sps
	pending        float64    // the proportional correction for the next step, as a fraction of sps
}

// NewSymbolSync returns a symbol synchronizer for c. The sample type cannot be inferred and has
// to be given, as in NewSymbolSync[float32](c).
func NewSymbolSync[T Sample](c SymbolSyncConfig) (*SymbolSync[T], error) {
	switch {
	case !(c.SamplesPerSymbol >= 2) || math.IsInf(c.SamplesPerSymbol, 1):
		return nil, errors.New("dsp: symbol sync needs at least 2 samples per symbol")
	case !(c.LoopBandwidth > 0 && c.LoopBandwidth < 0.5):
		return nil, errors.New("dsp: symbol sync loop bandwidth must be in (0, 0.5) cycles per symbol")
	case !(c.MaxDeviation >= 0 && c.MaxDeviation < 0.5):
		return nil, errors.New("dsp: symbol sync maximum deviation must be in [0, 0.5)")
	case c.Detector != Gardner && c.Detector != MuellerMuller:
		return nil, errors.New("dsp: unknown symbol sync timing detector")
	}
	alpha, beta := loopGains(c.LoopBandwidth)
	s := &SymbolSync[T]{
		sps:      c.SamplesPerSymbol,
		alpha:    alpha / symSyncDetectorGain,
		beta:     beta / symSyncDetectorGain,
		maxDev:   c.MaxDeviation,
		detector: c.Detector,
		// The proportional correction is limited to a quarter symbol either way.
		minStep: c.SamplesPerSymbol * (1 - c.MaxDeviation - 0.25),
	}
	s.Reset()
	return s, nil
}

// SamplesPerSymbol returns the samples per symbol the loop currently tracks.
func (s *SymbolSync[T]) SamplesPerSymbol() float64 { return s.sps * (1 - s.integ) }

// OutputLen returns the most samples Process can write for that many inputs. It depends on the
// position carried from earlier calls.
func (s *SymbolSync[T]) OutputLen(inputs int) int {
	// A symbol at idx needs the samples up to idx+2.
	room := float64(len(s.hist)+inputs-2-s.idx) - s.frac
	if room <= 0 {
		return 0
	}
	return int(math.Ceil(room / s.minStep))
}

// Process writes one sample per symbol whose center src reaches and returns how many it wrote,
// at most OutputLen(len(src)). It panics if dst is shorter than OutputLen(len(src)). dst may
// share memory with src, since src is copied before any output is written.
func (s *SymbolSync[T]) Process(dst, src []T) int {
	checkDst("SymbolSync", len(dst), s.OutputLen(len(src)))
	s.hist = append(s.hist, src...)
	n := 0
	for s.idx < len(s.hist)-2 {
		y := s.at(s.idx, s.frac)
		if s.started {
			var e float64
			switch s.detector {
			case Gardner:
				// The midpoint, measured from the previous symbol so that it rounds the same
				// wherever the two sit in hist.
				m := s.prevFrac + (float64(s.idx-s.prevIdx)+(s.frac-s.prevFrac))/2
				fm := math.Floor(m)
				mid := s.at(s.prevIdx+int(fm), m-fm)
				e = real((y - s.last) * cmplx.Conj(mid))
			case MuellerMuller:
				e = real(decide(y)*cmplx.Conj(s.last) - decide(s.last)*cmplx.Conj(y))
			}
			s.step(e, y)
		} else {
			s.power = real(y)*real(y) + imag(y)*imag(y)
			s.started = true
		}
		dst[n] = fromComplex128[T](y)
		n++
		s.last = y
		s.prevIdx, s.prevFrac = s.idx, s.frac
		f := s.frac + s.interval()
		k := math.Floor(f)
		s.idx += int(k)
		s.frac = f - k
	}
	// Keep from one sample before the previous symbol, which the next midpoint reads.
	if drop := s.prevIdx - 1; s.started && drop > 0 {
		drop = min(drop, len(s.hist))
		s.hist = append(s.hist[:0], s.hist[drop:]...)
		s.idx -= drop
		s.prevIdx -= drop
	}
	return n
}

// step updates the power estimate and the loop with the timing error e of symbol y. A
// positive e means the symbol was sampled late.
func (s *SymbolSync[T]) step(e float64, y complex128) {
	p := real(y)*real(y) + imag(y)*imag(y)
	s.power += symSyncPowerAlpha * (p - s.power)
	if s.power <= 0 {
		return
	}
	if s.detector == Gardner {
		e /= s.power
	} else {
		e /= math.Sqrt(s.power)
	}
	if math.IsNaN(e) || math.IsInf(e, 0) {
		return
	}
	s.integ = max(-s.maxDev, min(s.maxDev, s.integ+s.beta*e))
	s.pending = max(-0.25, min(0.25, s.alpha*e))
}

// interval returns the step to the next symbol, shortened when the clock runs late.
func (s *SymbolSync[T]) interval() float64 {
	v := s.sps * (1 - s.integ - s.pending)
	s.pending = 0
	return v
}

// at interpolates the input at hist index i plus mu, in [0, 1). The input needs a sample
// before i and two after it.
func (s *SymbolSync[T]) at(i int, mu float64) complex128 {
	// Four-point Lagrange coefficients for the points at -1, 0, 1, 2.
	cm := -mu * (mu - 1) * (mu - 2) / 6
	c0 := (mu + 1) * (mu - 1) * (mu - 2) / 2
	c1 := -(mu + 1) * mu * (mu - 2) / 2
	c2 := (mu + 1) * mu * (mu - 1) / 6
	h := s.hist[i-1 : i+3]
	return complex(cm, 0)*toComplex128(h[0]) + complex(c0, 0)*toComplex128(h[1]) +
		complex(c1, 0)*toComplex128(h[2]) + complex(c2, 0)*toComplex128(h[3])
}

// Reset returns the synchronizer to the start of a stream at the nominal rate, with the first
// symbol taken at the first input sample.
func (s *SymbolSync[T]) Reset() {
	// One sample of silence before the input gives the first symbol its point before.
	s.hist = append(s.hist[:0], 0)
	s.idx, s.prevIdx = 1, 1
	s.frac, s.prevFrac = 0, 0
	s.started, s.last, s.power, s.integ, s.pending = false, 0, 0, 0, 0
}

// decide returns the nearest point of a ±1 constellation on each axis, 0 on an axis whose part
// is 0.
func decide(v complex128) complex128 {
	return complex(sign(real(v)), sign(imag(v)))
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}
