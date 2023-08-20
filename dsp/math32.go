package dsp

import (
	"math"
	"unsafe"
)

// VMulC64xF32 multiplies a vector of complex values with a vector with real values.
// This is useful for applying a window to complex samples.
//
//	output[i] = complex(real(input[i])*mul[i], imag(input[i])*mul[i])
func VMulC64xF32(input, output []complex64, mul []float32)
func vMulC64xF32(input, output []complex64, mul []float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	if len(mul) < n {
		n = len(mul)
	}
	for i, v := range input[:n] {
		w := mul[i]
		output[i] = complex(real(v)*w, imag(v)*w)
	}
}

// VMulC64 multiplies eache value of the input by the matching value in the multiplier.
//
//	output[i] = input[i] * mul[i]
func VMulC64(input, output, mul []complex64) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	if len(mul) < n {
		n = len(mul)
	}
	for i, v := range input[:n] {
		output[i] = v * mul[i]
	}
}

func VAddF32(input, output []float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] += v
	}
}

func VAddC64(input, output []complex64) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] += v
	}
}

func VScaleC64(input, output []complex64, scale float32) {
	in := (*[2 << 25]float32)(unsafe.Pointer(&input[0]))[:len(input)*2]
	out := (*[2 << 25]float32)(unsafe.Pointer(&output[0]))[:len(output)*2]
	VScaleF32(in, out, scale)
}

func VScaleF32(input, output []float32, scale float32)
func vscaleF32(input, output []float32, scale float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] = v * scale
	}
}

func VAbsC64(input []complex64, output []float32)
func vAbsC64(input []complex64, output []float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	_ = output[n-1] // eliminate bounds check
	for i, v := range input[:n] {
		output[i] = float32(math.Sqrt(float64(real(v)*real(v) + imag(v)*imag(v))))
	}
}

// VMaxF32 returns the maximum value from an array of 32-bit floating point values.
func VMaxF32(input []float32) float32
func vMaxF32(input []float32) float32 {
	max := float32(math.Inf(-1))
	for _, v := range input {
		if v > max {
			max = v
		}
	}
	return max
}

// VMinF32 returns the minimum value from an array of 32-bit floating point values.
func VMinF32(input []float32) float32
func vMinF32(input []float32) float32 {
	min := float32(math.Inf(1))
	for _, v := range input {
		if v < min {
			min = v
		}
	}
	return min
}

func Conj32(x complex64) complex64    { return complex(real(x), -imag(x)) }
func FastPhase32(x complex64) float32 { return FastAtan2(imag(x), real(x)) }
func Phase32(x complex64) float32     { return float32(math.Atan2(float64(imag(x)), float64(real(x)))) }

const (
	pi2  = math.Pi / 2
	pi4  = math.Pi / 4
	pi34 = math.Pi * 3 / 4
)

// max |error| < 0.01
func FastAtan2(y, x float32) float32
func fastAtan2(y, x float32) float32 {
	absY := y
	if absY < 0 {
		absY = -absY
	}
	absY += 1e-20 // kludge to prevent 0/0 condition
	var angle float32
	if x < 0.0 {
		r := (x + absY) / (absY - x)
		angle = pi34 + (0.1963*r*r-0.9817)*r
	} else if x > 0.0 {
		r := (x - absY) / (x + absY)
		angle = pi4 + (0.1963*r*r-0.9817)*r
	} else if y < 0.0 {
		return -pi2
	} else if y > 0.0 {
		return pi2
	} else {
		return 0.0
	}
	if y < 0.0 {
		return -angle // negate if in quad III or IV
	}
	return angle
}

// |error| < 0.005
func FastAtan2_2(y, x float32) float32
func fastAtan2_2(y, x float32) float32 {
	if x == 0.0 {
		switch {
		case y > 0.0:
			return pi2
		case y < 0.0:
			return -pi2
		}
		return 0.0
	}
	z := y / x
	zz := z * z
	if zz < 1.0 {
		atan := z / (1.0 + 0.28*zz)
		if x < 0.0 {
			if y < 0.0 {
				return atan - math.Pi
			}
			return atan + math.Pi
		}
		return atan
	}
	atan := pi2 - z/(zz+0.28)
	if y < 0.0 {
		return atan - math.Pi
	}
	return atan
}
