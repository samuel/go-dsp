package sdr

import "math"

func Conj32(x complex64) complex64 { return complex(real(x), -imag(x)) }
func Phase32(x complex64) float32  { return float32(math.Atan2(float64(imag(x)), float64(real(x)))) }
