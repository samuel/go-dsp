package dsp

import (
	"errors"
	"math"
)

// AGCConfig sets how an AGC follows a signal's level.
type AGCConfig struct {
	// Target is the envelope amplitude the AGC holds the output at, such as 0.5 of full scale.
	Target float64
	// Attack and Decay are the time constants, in samples, with which the envelope follows a
	// rising and a falling level. A short attack keeps a sudden loud signal from overshooting;
	// a long decay keeps the gain from pumping between words or symbols.
	Attack, Decay float64
	// MaxGain caps the gain, as a linear factor of at least 1, so silence and noise are not
	// raised without limit.
	MaxGain float64
}

func (c AGCConfig) check() error {
	finitePos := func(v float64) bool { return v > 0 && !math.IsInf(v, 0) }
	switch {
	case !finitePos(c.Target):
		return errors.New("dsp: AGC target must be positive and finite")
	case !finitePos(c.Attack) || !finitePos(c.Decay):
		return errors.New("dsp: AGC attack and decay must be positive and finite")
	case !(c.MaxGain >= 1) || math.IsInf(c.MaxGain, 0):
		return errors.New("dsp: AGC max gain must be at least 1 and finite")
	}
	return nil
}

// AGC scales a signal so its envelope stays near a target. The envelope follows the sample
// magnitude, quickly when it rises and slowly when it falls, and the gain is the target over
// the envelope, capped at MaxGain. Its state is float64 at every width. Not safe for concurrent
// use.
type AGC[T Sample] struct {
	cfg           AGCConfig
	attack, decay float64 // per-sample smoothing coefficients
	env           float64 // envelope, in amplitude
}

// NewAGC returns an AGC whose envelope starts at the target, so the gain starts at 1.
func NewAGC[T Sample](cfg AGCConfig) (*AGC[T], error) {
	a := &AGC[T]{}
	if err := a.SetConfig(cfg); err != nil {
		return nil, err
	}
	a.Reset()
	return a, nil
}

// SetConfig changes the settings from the next sample on, keeping the envelope.
func (a *AGC[T]) SetConfig(cfg AGCConfig) error {
	if err := cfg.check(); err != nil {
		return err
	}
	a.cfg = cfg
	a.attack = 1 - math.Exp(-1/cfg.Attack)
	a.decay = 1 - math.Exp(-1/cfg.Decay)
	return nil
}

// Config returns the settings in use.
func (a *AGC[T]) Config() AGCConfig { return a.cfg }

// Gain returns the gain the next sample gets, before its own magnitude moves the envelope.
func (a *AGC[T]) Gain() float64 {
	return a.cfg.Target / max(a.env, a.cfg.Target/a.cfg.MaxGain)
}

// OutputLen returns inputs: the AGC writes one sample per input.
func (a *AGC[T]) OutputLen(inputs int) int { return inputs }

// Process writes src scaled to the target level to dst and returns len(src). It panics if dst
// is shorter than src. dst may be src.
func (a *AGC[T]) Process(dst, src []T) int {
	checkDst("AGC", len(dst), len(src))
	for i, x := range src {
		m := magnitude(x)
		k := a.decay
		if m > a.env {
			k = a.attack
		}
		a.env += k * (m - a.env)
		dst[i] = scaleSample(x, a.Gain())
	}
	return len(src)
}

// Reset sets the envelope back to the target, for a gain of 1.
func (a *AGC[T]) Reset() { a.env = a.cfg.Target }

// PowerSquelchConfig sets when a PowerSquelch opens.
type PowerSquelchConfig struct {
	// Threshold is the average power, in dB relative to a full-scale sample of magnitude 1,
	// at which the squelch opens.
	Threshold float64
	// Hysteresis is how far, in dB, the power must fall below Threshold before the squelch
	// closes again, so a signal near the threshold does not chatter. It must not be negative.
	Hysteresis float64
	// Average is the time constant, in samples, of the power average.
	Average float64
}

func (c PowerSquelchConfig) check() error {
	switch {
	case math.IsNaN(c.Threshold) || math.IsInf(c.Threshold, 0):
		return errors.New("dsp: squelch threshold must be finite")
	case !(c.Hysteresis >= 0) || math.IsInf(c.Hysteresis, 0):
		return errors.New("dsp: squelch hysteresis must be finite and not negative")
	case !(c.Average > 0) || math.IsInf(c.Average, 0):
		return errors.New("dsp: squelch average must be positive and finite")
	}
	return nil
}

// PowerSquelch passes a signal while its average power is above a threshold and writes zeros
// while it is below, with hysteresis between opening and closing. Not safe for concurrent use.
type PowerSquelch[T Sample] struct {
	cfg        PowerSquelchConfig
	k          float64 // averaging coefficient
	open, shut float64 // linear power thresholds
	power      float64 // average power, linear
	isOpen     bool
}

// NewPowerSquelch returns a closed squelch with no power measured.
func NewPowerSquelch[T Sample](cfg PowerSquelchConfig) (*PowerSquelch[T], error) {
	s := &PowerSquelch[T]{}
	if err := s.SetConfig(cfg); err != nil {
		return nil, err
	}
	return s, nil
}

// SetConfig changes the settings from the next sample on, keeping the average.
func (s *PowerSquelch[T]) SetConfig(cfg PowerSquelchConfig) error {
	if err := cfg.check(); err != nil {
		return err
	}
	s.cfg = cfg
	s.k = 1 - math.Exp(-1/cfg.Average)
	s.open = math.Pow(10, cfg.Threshold/10)
	s.shut = math.Pow(10, (cfg.Threshold-cfg.Hysteresis)/10)
	return nil
}

// Config returns the settings in use.
func (s *PowerSquelch[T]) Config() PowerSquelchConfig { return s.cfg }

// Open reports whether the squelch passed the last sample.
func (s *PowerSquelch[T]) Open() bool { return s.isOpen }

// Power returns the average power in dB relative to full scale. It is -Inf before any signal.
func (s *PowerSquelch[T]) Power() float64 { return 10 * math.Log10(s.power) }

// OutputLen returns inputs: the squelch writes one sample per input.
func (s *PowerSquelch[T]) OutputLen(inputs int) int { return inputs }

// Process writes src to dst while the squelch is open and zeros while it is closed, and
// returns len(src). It panics if dst is shorter than src. dst may be src.
func (s *PowerSquelch[T]) Process(dst, src []T) int {
	checkDst("PowerSquelch", len(dst), len(src))
	for i, x := range src {
		m := magnitude(x)
		s.power += s.k * (m*m - s.power)
		switch {
		case !s.isOpen && s.power >= s.open:
			s.isOpen = true
		case s.isOpen && s.power < s.shut:
			s.isOpen = false
		}
		if s.isOpen {
			dst[i] = x
		} else {
			dst[i] = 0
		}
	}
	return len(src)
}

// Reset closes the squelch and clears the average.
func (s *PowerSquelch[T]) Reset() {
	s.power = 0
	s.isOpen = false
}

// magnitude returns |x| in float64.
func magnitude[T Sample](x T) float64 {
	switch v := any(x).(type) {
	case float32:
		return math.Abs(float64(v))
	case float64:
		return math.Abs(v)
	case complex64:
		return math.Hypot(float64(real(v)), float64(imag(v)))
	case complex128:
		return math.Hypot(real(v), imag(v))
	}
	return 0
}

// scaleSample returns x times the real gain g, formed in float64.
func scaleSample[T Sample](x T, g float64) T {
	var out any
	switch v := any(x).(type) {
	case float32:
		out = float32(float64(v) * g)
	case float64:
		out = v * g
	case complex64:
		out = complex(float32(float64(real(v))*g), float32(float64(imag(v))*g))
	case complex128:
		out = complex(real(v)*g, imag(v)*g)
	}
	return out.(T)
}
