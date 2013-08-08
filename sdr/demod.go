package sdr

import (
	"math"
	"math/cmplx"
)

func PolarDiscriminator(a, b complex128) float64 {
	return cmplx.Phase(a*cmplx.Conj(b)) / math.Pi
}

func PolarDiscriminator32(a, b complex64) float32 {
	return FastPhase32(a * Conj32(b)) // / math.Pi
}

type FMDemodFilter struct {
	pre complex64
}

func (fi *FMDemodFilter) Demodulate(input []complex64, output []float32) (int, error) {
	// 	return fmDemodulateAsm(fi, input, output)
	// }

	// func fmDemodulateAsm(fi *FMDemodFilter, input []complex64, output []float32) (int, error)

	// func fmDemodulate(fi *FMDemodFilter, input []complex64, output []float32) (int, error) {
	for i := 0; i < len(input); i++ {
		inp := input[i]
		output[i] = PolarDiscriminator32(inp, fi.pre)
		fi.pre = inp
	}
	return len(input), nil
}
