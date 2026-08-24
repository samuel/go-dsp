package dsp

import "math"

// Scalar complex helpers, standing in for the parts of math/cmplx that this
// package needs at both widths.
//
// They route through complex128 because the real and imag builtins do not
// accept an argument of type-parameter type
// (https://github.com/golang/go/issues/45049).

// Conj returns the complex conjugate of x.
func Conj[C Complex](x C) C {
	z := complex128(x)
	return C(complex(real(z), -imag(z)))
}

// Phase returns the phase angle of x, in the range [-Pi, Pi]. It computes the
// angle in float64 and returns float64 at both widths, since a result type that
// appears nowhere in the arguments could not be inferred. It is exact but much
// slower than FastPhase.
func Phase[C Complex](x C) float64 {
	z := complex128(x)
	return math.Atan2(imag(z), real(z))
}
