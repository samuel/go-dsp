package dsp

import "samuelks.com/dsp/internal/f32"

// FastPhase returns an approximation of the phase angle of x, in the range
// [-Pi, Pi]. It uses FastAtan2, so see that function for the error bound.
func FastPhase(x complex64) float32 { return FastAtan2(imag(x), real(x)) }

// FastAtan2 returns an approximation of Atan2(y, x), the angle of the vector
// (x, y), in the range [-Pi, Pi]. The relative error is under 2.2e-4 at every
// angle and the absolute error under 1.7e-4 radians, so a small angle is not
// biased toward zero, which matters to an FM discriminator.
//
// It costs one division and a degree-7 polynomial. Unlike Atan2, a zero y
// returns +0 or +Pi whatever its sign, two infinities return NaN rather than
// a multiple of Pi/4, and a NaN in either argument returns NaN.
func FastAtan2(y, x float32) float32 { return f32.FastAtan2(y, x) }
