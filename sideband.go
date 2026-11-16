package dsp

import (
	"errors"
	"math"
)

// sidebandAttenuation is the stopband of a SidebandDemod's filter, in dB.
const sidebandAttenuation = 60

// SidebandDemod demodulates one band of a complex signal to a real one: it keeps the
// frequencies from Low to High, shifts them by Shift, and takes the real part. With the carrier
// at 0 Hz this is SSB, the band above 0 Hz for upper sideband and below for lower; with a band
// around 0 Hz and a Shift of the desired pitch it is CW, the beat-frequency oscillator turning
// the carrier into a tone. Frequencies are in cycles per sample. Not safe for concurrent use.
//
// The band-pass is a real low-pass filter centered on the band by mixing down to it and back
// up, with the second mix also applying Shift. The second mix starts behind by the filter's
// delay, so with no Shift the output is the real part of the band, delayed by (len(taps)-1)/2
// samples.
type SidebandDemod[C Complex, F Float] struct {
	down, up phasor
	upStart  float64 // the up mixer's starting phase, in cycles
	filter   *FIRFilter[C]
	work     []C
}

// NewSidebandDemod returns a demodulator for the band from low to high, low < high, with
// transition bands of width transition, which sets the filter's length. It shifts the band by
// shift.
func NewSidebandDemod[C Complex, F Float](low, high, transition, shift float64) (*SidebandDemod[C, F], error) {
	switch {
	case !(low < high) || low < -0.5 || high > 0.5:
		return nil, errors.New("dsp: sideband must have low < high within [-0.5, 0.5] cycles per sample")
	case !(transition > 0 && transition < 0.5):
		return nil, errors.New("dsp: sideband transition must be in (0, 0.5) cycles per sample")
	case math.IsNaN(shift) || math.IsInf(shift, 0):
		return nil, errors.New("dsp: sideband shift must be finite")
	}
	center, half := (low+high)/2, (high-low)/2
	// Kaiser's estimate of the length for the attenuation, made odd so the delay is whole.
	n := int(math.Ceil((sidebandAttenuation-7.95)/(14.36*transition))) + 1 | 1
	taps, err := LowPassFIR[float64](n, min(half+transition/2, 0.5), KaiserBeta(sidebandAttenuation))
	if err != nil {
		return nil, err
	}
	filter, err := NewRealCoefFIRFilter[C](taps)
	if err != nil {
		return nil, err
	}
	down, err := newPhasor(-center)
	if err != nil {
		return nil, err
	}
	up, err := newPhasor(center + shift)
	if err != nil {
		return nil, err
	}
	delay := float64(n / 2)
	d := &SidebandDemod[C, F]{down: down, up: up, filter: filter}
	d.upStart = -center * delay
	d.upStart -= math.Floor(d.upStart)
	d.up.phase = d.upStart
	return d, nil
}

// OutputLen returns inputs: the demodulator writes one sample per input.
func (d *SidebandDemod[C, F]) OutputLen(inputs int) int { return inputs }

// Process writes the demodulated band to dst and returns len(src). It panics if dst is
// shorter than src.
func (d *SidebandDemod[C, F]) Process(dst []F, src []C) int {
	checkDst("SidebandDemod", len(dst), len(src))
	if cap(d.work) < len(src) {
		d.work = make([]C, len(src))
	}
	w := d.work[:len(src)]
	for i, x := range src {
		s, c := d.down.next()
		w[i] = C(complex128(x) * complex(c, s))
	}
	d.filter.Process(w, w)
	for i, v := range w {
		s, c := d.up.next()
		// The real part of v·e^{jφ}.
		dst[i] = F(real(complex128(v))*c - imag(complex128(v))*s)
	}
	return len(src)
}

// Reset clears the filter and returns both mixers to phase zero.
func (d *SidebandDemod[C, F]) Reset() {
	d.filter.Reset()
	d.down.phase, d.up.phase = 0, d.upStart
}
