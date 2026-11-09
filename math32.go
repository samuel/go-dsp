package dsp

import (
	"math"
)

// FastPhase returns an approximation of the phase angle of x, in the range
// [-Pi, Pi]. It uses FastAtan2, so see that function for the error bound.
func FastPhase(x complex64) float32 { return FastAtan2(imag(x), real(x)) }

const (
	pi2  = math.Pi / 2
	pi4  = math.Pi / 4
	pi34 = math.Pi * 3 / 4
)

// FastAtan2 returns an approximation of Atan2(y, x), the angle of the vector
// (x, y), in the range [-Pi, Pi]. The maximum absolute error is a little over
// 0.01 radians. See FastAtan2Fine for a more accurate approximation.
//
// A zero x is tested before anything is divided, so a NaN x -- which is
// neither negative nor positive -- takes the same branch and returns the angle
// of the y axis: +Pi/2, -Pi/2, or +0 when y is zero or itself NaN. A NaN y with
// a nonzero x returns NaN. The result is +0 rather than -0 for every input that
// lands on zero, including a negative-zero y.
func FastAtan2(y, x float32) float32 { return fastAtan2Asm(y, x) }
func fastAtan2(y, x float32) float32 {
	absY := max(y, -y)
	absY += 1e-20 // kludge to prevent 0/0 condition
	var angle float32
	switch {
	case x < 0.0:
		r := (x + absY) / (absY - x)
		angle = pi34 + float32(quadPoly(r)*r)
	case x > 0.0:
		r := (x - absY) / (x + absY)
		angle = pi4 + float32(quadPoly(r)*r)
	case y < 0.0:
		return -pi2
	case y > 0.0:
		return pi2
	default:
		return 0.0
	}
	if y < 0.0 {
		return -angle // negate if in quad III or IV
	}
	return angle
}

// quadPoly is 0.1963*r*r - 0.9817, the odd part of FastAtan2's quadrant
// polynomial, with the product rounded before the subtraction.
//
// The explicit conversion is needed to ensure that arm64 does not change
// the multiply and the subtraction into one FMSUB, which the unfused
// MULF/SUBF pair in math32_arm.s cannot reproduce, and the two architectures
// then disagree on the low bit of most inputs. Its caller rounds
// the outer product for the same reason -- FMADD with pi34 or pi4.
func quadPoly(r float32) float32 { return float32(0.1963*r*r) - 0.9817 }

// FastAtan2Fine returns an approximation of Atan2(y, x), the angle of the
// vector (x, y), in the range [-Pi, Pi]. The maximum absolute error is under
// 0.005 radians, twice as accurate as FastAtan2 and, on most targets, no
// slower: it divides once instead of folding the ratio into a quadrant, so
// which of the two wins depends on the divider.
//
// Unlike FastAtan2 it propagates NaN: only a zero x is special-cased, so a NaN
// in either argument reaches the division and comes back out. A zero x returns
// +Pi/2, -Pi/2, or +0 when y is zero or NaN -- +0 rather than -0 even for a
// negative-zero y.
func FastAtan2Fine(y, x float32) float32 { return fastAtan2FineAsm(y, x) }
func fastAtan2Fine(y, x float32) float32 {
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
	// Rounded explicitly, so that z*z cannot contract with the + 0.28 below
	// into an FMADD that math32_arm.s's separate MULF and ADDF do not produce.
	zz := float32(z * z)
	if zz < 1.0 {
		atan := z / (1.0 + float32(0.28*zz))
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
