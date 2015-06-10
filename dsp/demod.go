package dsp

import "math/cmplx"

// PolarDiscriminator returns the phase angle between two complex vectors
// equivalent to arg(a * conj(b)). The returned angle is in the range [-Pi, Pi].
func PolarDiscriminator(a, b complex128) float64 {
	return cmplx.Phase(a * cmplx.Conj(b))
}

// PolarDiscriminator32 returns the phase angle between two complex vectors
// equivalent to arg(a * conj(b)). The returned angle is in the range [-Pi, Pi].
func PolarDiscriminator32(a, b complex64) float32 {
	return FastAtan2(imag(a)*real(b)-real(a)*imag(b), real(a)*real(b)+imag(a)*imag(b))
}

// FMDemodFilter is an FM demodulator filter using a polar disciminator.
//
// 	x(n)─────▶○───────────────────▶(X)──────────────────▶arctan──▶
// 	          │                     ▲  y(n)=x(n)x*(n-1)
// 	          │   ┌───┐     ┌───┐   │
// 	          └──▶│z⁻¹├────▶│z^*├───┘
// 	              └───┘     └───┘
type FMDemodFilter struct {
	pre complex64
}

func (fi *FMDemodFilter) Demodulate(input []complex64, output []float32) int {
	return fmDemodulateAsm(fi, input, output)
}

func fmDemodulateAsm(fi *FMDemodFilter, input []complex64, output []float32) int

func fmDemodulate(fi *FMDemodFilter, input []complex64, output []float32) int {
	pre := fi.pre
	for i, inp := range input {
		// output[i] = PolarDiscriminator32(inp, pre)
		output[i] = FastAtan2(imag(inp)*real(pre)-real(inp)*imag(pre), real(inp)*real(pre)+imag(inp)*imag(pre))
		pre = inp
	}
	fi.pre = pre
	return len(input)
}
