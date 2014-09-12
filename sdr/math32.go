package sdr

import "math"

func Scalef32(input, output []float32, scale float32)

func scalef32(input, output []float32, scale float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] = v * scale
	}
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
	} else {
		atan := pi2 - z/(zz+0.28)
		if y < 0.0 {
			return atan - math.Pi
		}
		return atan
	}
}
