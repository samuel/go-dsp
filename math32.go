package dsp

import "samuelks.com/dsp/internal/f32"

// FastPhase returns an approximation of the phase angle of x, in the range
// [-Pi, Pi]. It uses FastAtan2, so see that function for the error bound.
func FastPhase(x complex64) float32 { return FastAtan2(imag(x), real(x)) }

// FastAtan2 returns an approximation of Atan2(y, x), the angle of the vector
// (x, y), in the range [-Pi, Pi]. The maximum absolute error is a little over
// 0.01 radians. See FastAtan2Fine for a more accurate approximation.
//
// A zero x is tested before anything is divided, so a NaN x -- which is
// neither negative nor positive -- takes the same branch and returns the angle
// of the y axis: +Pi/2, -Pi/2, or +0 when y is zero or itself NaN. A NaN y with
// a nonzero x returns NaN. The result is +0 rather than -0 for every input that
// lands on zero, including a negative-zero y.
func FastAtan2(y, x float32) float32 { return f32.FastAtan2(y, x) }

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
func FastAtan2Fine(y, x float32) float32 { return f32.FastAtan2Fine(y, x) }
