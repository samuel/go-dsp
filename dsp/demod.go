package dsp

import "math/cmplx"

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
	// Each product is rounded before it is added or subtracted. Without that,
	// arm64 contracts the two pairs into FMSUB and FMADD, which the four separate
	// MULF instructions in demod_arm.s cannot reproduce; see the note on quadPoly.
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
type FMDemod struct {
	pre complex64
}

// NewFMDemod returns an FM demodulator that starts from silence. The zero
// value is equivalent and equally usable; the constructor is here so that every
// stateful type in the package has one.
func NewFMDemod() *FMDemod {
	return &FMDemod{}
}

// Demodulate writes one output sample per input sample, over
// min(len(src), len(dst)) of them. The filter carries the previous sample
// across calls, so a stream can be demodulated in blocks.
// It returns nothing; the count is the min of the two lengths.
func (f *FMDemod) Demodulate(dst []float32, src []complex64) {
	fmDemodulateAsm(f, dst, src)
}

// Reset drops the carried sample, so the demodulator starts a new stream
// without a phase step at the join.
func (f *FMDemod) Reset() {
	f.pre = 0
}

// Demodulating is FastPolarDiscriminator per sample against the previous one;
// its explicit roundings must not be removed.
func fmDemodulate(f *FMDemod, dst []float32, src []complex64) {
	n := min(len(src), len(dst))
	pre := f.pre
	for i, inp := range src[:n] {
		dst[i] = FastPolarDiscriminator(inp, pre)
		pre = inp
	}
	f.pre = pre
}
