package sdr

import "math"

func Conj32(x complex64) complex64    { return complex(real(x), -imag(x)) }
func FastPhase32(x complex64) float32 { return FastAtan2(imag(x), real(x)) }
func Phase32(x complex64) float32     { return float32(math.Atan2(float64(imag(x)), float64(real(x)))) }

const (
	pi2  = math.Pi / 2
	pi4  = math.Pi / 4
	pi34 = math.Pi * 3 / 4
)

// max |error| < 0.01
func FastAtan2(y, x float32) float32 {
	absY := y
	if absY < 0 {
		absY = -absY
	}
	absY += 1e-10 // kludge to prevent 0/0 condition
	var r, angle float32
	if x < 0.0 {
		r = (x + absY) / (absY - x)
		angle = pi34
	} else {
		r = (x - absY) / (x + absY)
		angle = pi4
	}
	angle += (0.1963*r*r - 0.9817) * r
	if y < 0.0 {
		return -angle // negate if in quad III or IV
	}
	return angle
}

// |error| < 0.005
func FastAtan2_2(y, x float32) float32 {
	if x == 0.0 {
		if y > 0.0 {
			return pi2
		}
		if y == 0.0 {
			return 0.0
		}
		return -pi2
	}
	z := y / x
	absZ := z
	if absZ < 0 {
		absZ = -absZ
	}
	if absZ < 1.0 {
		atan := z / (1.0 + 0.28*z*z)
		if x < 0.0 {
			if y < 0.0 {
				return atan - math.Pi
			}
			return atan + math.Pi
		}
		return atan
	} else {
		atan := pi2 - z/(z*z+0.28)
		if y < 0.0 {
			return atan - math.Pi
		}
		return atan
	}
}
