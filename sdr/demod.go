package sdr

import (
	"math"
	"math/cmplx"
)

func PolarDiscriminator(a, b complex128) float64 {
	return cmplx.Phase(a*cmplx.Conj(b)) / math.Pi
}

func PolarDiscriminator32(a, b complex64) float32 {
	// return FastPhase32(a * Conj32(b)) // / math.Pi
	return FastAtan2(imag(a)*real(b)-real(a)*imag(b), real(a)*real(b)+imag(a)*imag(b))
}

type FMDemodFilter struct {
	pre complex64
}

func (fi *FMDemodFilter) Demodulate(input []complex64, output []float32) (int, error) {
	return fmDemodulateAsm(fi, input, output)
}

func fmDemodulateAsm(fi *FMDemodFilter, input []complex64, output []float32) (int, error)

func fmDemodulate(fi *FMDemodFilter, input []complex64, output []float32) (int, error) {
	pre := fi.pre
	for i, inp := range input {
		// output[i] = PolarDiscriminator32(inp, pre)
		output[i] = FastAtan2(imag(inp)*real(pre)-real(inp)*imag(pre), real(inp)*real(pre)+imag(inp)*imag(pre))
		pre = inp
	}
	fi.pre = pre
	return len(input), nil
}

// type I32FMDemodFilter struct {
// 	preR, preI int32
// }

// func (fi *I32FMDemodFilter) Demodulate(input []int32, output []int32) (int, error) {
// 	// 	return i32FMDemodulateAsm(fi, input, output)
// 	// }

// 	// func i32FMDemodulateAsm(fi *I32FMDemodFilter, input []complex64, output []float32) (int, error)

// 	// func i32FMDemodulate(fi *I32FMDemodFilter, input []complex64, output []float32) (int, error) {
// 	pre := fi.pre
// 	for i, inp := range input {
// 		// output[i] = PolarDiscriminator32(inp, pre)
// 		output[i] = FastAtan2Fixed(imag(inp)*real(pre)-real(inp)*imag(pre), real(inp)*real(pre)+imag(inp)*imag(pre))
// 		pre = inp
// 	}
// 	fi.pre = pre
// 	return len(input), nil
// }
