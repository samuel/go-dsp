package dsp

import (
	"slices"

	"samuelks.com/dsp/internal/f32"
	"samuelks.com/dsp/internal/f64"
)

// coefs is a coefficient set held in one of two forms, the other one nil:
// native, at the sample type, or real, for a complex sample type built by a
// NewRealCoef constructor. The real form is at the sample's component width,
// []float32 for complex64 and []float64 for complex128, so a complex signal
// with real coefficients runs as two real channels, with no complex multiply.
type coefs[T Sample] struct {
	native []T
	real   any
}

// realCoefs returns x as real coefficients for the complex sample type C.
func realCoefs[C Complex, F Float](x []F) coefs[C] {
	switch any(*new(C)).(type) {
	case complex64:
		return coefs[C]{real: convertFloats[float32](x)}
	case complex128:
		return coefs[C]{real: convertFloats[float64](x)}
	}
	panic("dsp: realCoefs: no case for this width")
}

// convertFloats returns a copy of x at width E.
func convertFloats[E, F Float](x []F) []E {
	out := make([]E, len(x))
	for i, v := range x {
		out[i] = E(v)
	}
	return out
}

func (c *coefs[T]) len() int {
	switch r := c.real.(type) {
	case []float32:
		return len(r)
	case []float64:
		return len(r)
	}
	return len(c.native)
}

// widen returns a copy of the coefficients at the sample type.
func (c *coefs[T]) widen() []T {
	if c.native != nil {
		return slices.Clone(c.native)
	}
	out := make([]T, c.len())
	switch r := c.real.(type) {
	case []float32:
		for i, v := range r {
			out[i] = realSample[T](float64(v))
		}
	case []float64:
		for i, v := range r {
			out[i] = realSample[T](v)
		}
	}
	return out
}

// float64s returns a copy of the coefficients in float64, or false if any has
// a non-zero imaginary part.
func (c *coefs[T]) float64s() ([]float64, bool) {
	var r any = c.native
	if c.native == nil {
		r = c.real
	}
	switch r := r.(type) {
	case []float32:
		return convertFloats[float64](r), true
	case []float64:
		return slices.Clone(r), true
	case []complex64:
		return realParts(r)
	case []complex128:
		return realParts(r)
	}
	return nil, false
}

// realParts returns the real parts of x in float64, or false if any imaginary
// part is non-zero.
func realParts[C Complex](x []C) ([]float64, bool) {
	out := make([]float64, len(x))
	for i, v := range x {
		z := complex128(v)
		if imag(z) != 0 {
			return nil, false
		}
		out[i] = real(z)
	}
	return out, true
}

// polyphase splits the coefficients into factor branches, branch p holding
// coefficients p, p+factor, p+2*factor, ..., zero-padded to a common length.
func (c *coefs[T]) polyphase(factor int) []coefs[T] {
	out := make([]coefs[T], factor)
	switch r := c.real.(type) {
	case []float32:
		for p, b := range polyphase(r, factor) {
			out[p].real = b
		}
	case []float64:
		for p, b := range polyphase(r, factor) {
			out[p].real = b
		}
	default:
		for p, b := range polyphase(c.native, factor) {
			out[p].native = b
		}
	}
	return out
}

func polyphase[E any](taps []E, factor int) [][]E {
	n := (len(taps) + factor - 1) / factor
	all := make([]E, n*factor)
	branches := make([][]E, factor)
	for p := range branches {
		b := all[p*n : (p+1)*n : (p+1)*n]
		for k := range b {
			if j := p + k*factor; j < len(taps) {
				b[k] = taps[j]
			}
		}
		branches[p] = b
	}
	return branches
}

// fir writes the outputs of the FIR filter with these coefficients as taps
// to dst: dst[k] = sum over j of taps[j]*work[k*stride+len(taps)-1-j], with
// the kernel's rounding. work holds len(taps)-1 samples of history in front of
// the first output's sample.
func (c *coefs[T]) fir(dst, work []T, stride int) {
	switch r := c.real.(type) {
	case []float32:
		f32.CFIRReal(any(dst).([]complex64), any(work).([]complex64), r, stride)
		return
	case []float64:
		f64.CFIRReal(any(dst).([]complex128), any(work).([]complex128), r, stride)
		return
	}
	switch t := any(c.native).(type) {
	case []float32:
		f32.FIR(any(dst).([]float32), any(work).([]float32), t, stride)
	case []float64:
		f64.FIR(any(dst).([]float64), any(work).([]float64), t, stride)
	case []complex64:
		f32.CFIR(any(dst).([]complex64), any(work).([]complex64), t, stride)
	case []complex128:
		f64.CFIR(any(dst).([]complex128), any(work).([]complex128), t, stride)
	default:
		panic("dsp: FIR: no kernel for this width")
	}
}

// iirReal runs the direct form 1 recursion of IIRFilter.step with real b and a
// over a complex signal, with dst, src and the delay lines the interleaved
// views of complex slices.
func iirReal[E Float](dst, src, b, a, pIn, pOut []E) {
	for i := 0; i+1 < len(src); i += 2 {
		xr, xi := src[i], src[i+1]
		sr, si := b[0]*xr, b[0]*xi
		for j := range len(b) - 1 {
			sr += b[j+1]*pIn[2*j] - a[j+1]*pOut[2*j]
			si += b[j+1]*pIn[2*j+1] - a[j+1]*pOut[2*j+1]
		}
		copy(pIn[2:], pIn)
		copy(pOut[2:], pOut)
		pIn[0], pIn[1] = xr, xi
		pOut[0], pOut[1] = sr, si
		dst[i], dst[i+1] = sr, si
	}
}
