package dsp

import (
	"math/cmplx"

	"samuelks.com/dsp/internal/f32"
)

// PolarDiscriminator returns the phase angle between two complex vectors
// equivalent to arg(a * conj(b)). The returned angle is in the range [-Pi, Pi].
//
// It evaluates in float64 and returns float64 at both widths.
// FastPolarDiscriminator is the float32 approximation.
func PolarDiscriminator[C Complex](a, b C) float64 {
	x, y := complex128(a), complex128(b)
	return cmplx.Phase(x * cmplx.Conj(y))
}

// FastPolarDiscriminator returns an approximation of the phase angle between
// two complex vectors, equivalent to arg(a * conj(b)), in the range [-Pi, Pi].
//
// This is not a complex64 version of PolarDiscriminator but a different function: it
// is built on FastAtan2 and inherits that function's error bound, which is a
// float32 statement. That is also why it takes complex64 rather than being
// generic over Complex.
func FastPolarDiscriminator(a, b complex64) float32 {
	// Each product is rounded before it is added or subtracted, as in the
	// demodulator kernel, so the two agree on every architecture.
	return FastAtan2(
		float32(imag(a)*real(b))-float32(real(a)*imag(b)),
		float32(real(a)*real(b))+float32(imag(a)*imag(b)),
	)
}

// FMDemod is an FM demodulator filter using a polar discriminator.
//
//	x(n)─────▶○───────────────────▶(X)──────────────────▶arctan──▶
//	          │                     ▲  y(n)=x(n)x*(n-1)
//	          │   ┌───┐     ┌───┐   │
//	          └──▶│z⁻¹├────▶│z^*├───┘
//	              └───┘     └───┘
//
// The arithmetic happens at C's width and the result is converted to F, as in
// VAbs. complex64 uses FastPolarDiscriminator and inherits FastAtan2's error
// bound; FMDemod[complex64, float32] runs a kernel with VFP assembly on 32-bit
// arm. complex128 uses the
// exact PolarDiscriminator.
type FMDemod[C Complex, F Float] struct {
	pre C
}

// NewFMDemod returns an FM demodulator that starts from silence. The zero
// value is equivalent and equally usable; the constructor is here so that every
// stateful type in the package has one. The types cannot be inferred and have
// to be given, as in NewFMDemod[complex64, float32]().
func NewFMDemod[C Complex, F Float]() *FMDemod[C, F] {
	return &FMDemod[C, F]{}
}

// OutputLen returns inputs: the demodulator writes one sample per input.
func (f *FMDemod[C, F]) OutputLen(inputs int) int { return inputs }

// Process writes one output sample per input sample and returns len(src). It
// panics if dst is shorter than src. The filter carries the previous sample
// across calls, so a stream can be demodulated in blocks.
func (f *FMDemod[C, F]) Process(dst []F, src []C) int {
	checkDst("FMDemod", len(dst), len(src))
	dst = dst[:len(src)]
	if g, ok := any(f).(*FMDemod[complex64, float32]); ok {
		g.pre = f32.FMDemod(any(dst).([]float32), any(src).([]complex64), g.pre)
		return len(src)
	}
	_, single := any(src).([]complex64)
	pre := f.pre
	for i, inp := range src {
		var d float64
		if single {
			d = float64(FastPolarDiscriminator(complex64(inp), complex64(pre)))
		} else {
			d = PolarDiscriminator(inp, pre)
		}
		dst[i] = F(d)
		pre = inp
	}
	f.pre = pre
	return len(src)
}

// Reset drops the carried sample, so the demodulator starts a new stream
// without a phase step at the join.
func (f *FMDemod[C, F]) Reset() {
	f.pre = 0
}
