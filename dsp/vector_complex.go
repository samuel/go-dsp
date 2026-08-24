package dsp

import (
	"math"

	"github.com/samuel/go-dsp/dsp/f32"
	"github.com/samuel/go-dsp/dsp/f64"
	"github.com/samuel/go-dsp/dsp/internal/view"
)

// Elementwise vector arithmetic over complex samples. Every name is the real
// one from vector.go with C inserted after the V, and that file's rules apply
// unchanged; Real on the end means the other operand is real.
//
// The kernels are the real kernels over a reinterpreted slice -- complex64 is
// two float32 with no padding, complex128 two float64, and archsimd has no
// complex vector types. view.C64ToF32 and view.C128ToF64 are that view. The length
// contract survives it, since min(2a, 2b) is 2*min(a, b).

// VCAdd writes dst[i] = a[i] + b[i], over min(len(dst), len(a), len(b))
// elements. Any of the slices may be the same, so VCAdd(x, x, y) accumulates y
// into x.
func VCAdd[C Complex](dst, a, b []C) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v + b[i]
	}
}

// VCSub writes dst[i] = a[i] - b[i], under VCAdd's length and aliasing rules.
func VCSub[C Complex](dst, a, b []C) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v - b[i]
	}
}

// VCMul writes dst[i] = a[i] * b[i], the complex product, under VCAdd's length
// and aliasing rules.
func VCMul[C Complex](dst, a, b []C) {
	switch d := any(dst).(type) {
	case []complex64:
		vcmulC64(d, any(a).([]complex64), any(b).([]complex64))
	case []complex128:
		vcmulC128(d, any(a).([]complex128), any(b).([]complex128))
	default:
		panic("dsp: VCMul: no kernel for this width")
	}
}

// vcmulC64 is the complex64 product with every intermediate rounded.
//
// As vabsC64Scalar explains: written as xr*yr-xi*yi, Go may contract into an
// FMA and arm64 does,
// while VFMADDSUB fuses on amd64 and VMULPS+VSUBPS and 32-bit arm's VMLA do
// not. Pinning the roundings is what lets a vector path match. It is also why
// this is not the one-line generic body VCAdd gets.
func vcmulC64(dst, a, b []complex64) {
	n := min(len(dst), len(a), len(b))
	for i, x := range a[:n] {
		y := b[i]
		xr, xi, yr, yi := real(x), imag(x), real(y), imag(y)
		dst[i] = complex(
			float32(xr*yr)-float32(xi*yi),
			float32(xr*yi)+float32(xi*yr),
		)
	}
}

// vcmulC128 is vcmulC64 at the wider width; see the note there.
func vcmulC128(dst, a, b []complex128) {
	n := min(len(dst), len(a), len(b))
	for i, x := range a[:n] {
		y := b[i]
		xr, xi, yr, yi := real(x), imag(x), real(y), imag(y)
		dst[i] = complex(
			float64(xr*yr)-float64(xi*yi),
			float64(xr*yi)+float64(xi*yr),
		)
	}
}

// VCScale writes dst[i] = src[i] * k for a complex k, which rotates as well as
// scales. It processes min(len(dst), len(src)) elements and the slices may be
// the same. Use VCScaleReal for a real gain: that is a plain per-lane multiply
// with no rotation and reaches the real kernel directly.
func VCScale[C Complex](dst, src []C, k C) {
	switch d := any(dst).(type) {
	case []complex64:
		vcscaleC64(d, any(src).([]complex64), any(k).(complex64))
	case []complex128:
		vcscaleC128(d, any(src).([]complex128), any(k).(complex128))
	default:
		panic("dsp: VCScale: no kernel for this width")
	}
}

// vcscaleC64 rounds its intermediates for the reason vcmulC64 does.
func vcscaleC64(dst, src []complex64, k complex64) {
	n := min(len(dst), len(src))
	kr, ki := real(k), imag(k)
	for i, x := range src[:n] {
		xr, xi := real(x), imag(x)
		dst[i] = complex(
			float32(xr*kr)-float32(xi*ki),
			float32(xr*ki)+float32(xi*kr),
		)
	}
}

// vcscaleC128 is vcscaleC64 at the wider width.
func vcscaleC128(dst, src []complex128, k complex128) {
	n := min(len(dst), len(src))
	kr, ki := real(k), imag(k)
	for i, x := range src[:n] {
		xr, xi := real(x), imag(x)
		dst[i] = complex(
			float64(xr*kr)-float64(xi*ki),
			float64(xr*ki)+float64(xi*kr),
		)
	}
}

// VCScaleReal scales the real and imaginary parts of every sample by the same
// real factor, over min(len(dst), len(src)) elements. The slices may be the
// same.
//
// The scalar is generic over its own width, as VCMulReal's slice of them is, so
// the two members of the Real family read the same way. It is converted to C's
// component width on the way in either way, so F changes nothing but what the
// caller may hand over without a conversion of its own.
//
// One consequence: an untyped integer literal will not do, since inference
// gives it its default type and int does not satisfy Float. Write the factor as
// 2.0 rather than 2.
func VCScaleReal[C Complex, F Float](dst, src []C, k F) {
	switch d := any(dst).(type) {
	case []complex64:
		s := any(src).([]complex64)
		f32.Scale(view.C64ToF32(d), view.C64ToF32(s), float32(k))
	case []complex128:
		s := any(src).([]complex128)
		f64.Scale(view.C128ToF64(d), view.C128ToF64(s), float64(k))
	default:
		panic("dsp: VCScaleReal: no kernel for this width")
	}
}

// VCConj writes dst[i] = conj(src[i]), over min(len(dst), len(src)) elements.
// The slices may be the same.
func VCConj[C Complex](dst, src []C) {
	switch d := any(dst).(type) {
	case []complex64:
		s := any(src).([]complex64)
		n := min(len(d), len(s))
		conjF32(view.C64ToF32(d[:n]), view.C64ToF32(s[:n]))
	case []complex128:
		s := any(src).([]complex128)
		n := min(len(d), len(s))
		conjF64(view.C128ToF64(d[:n]), view.C128ToF64(s[:n]))
	default:
		panic("dsp: VCConj: no kernel for this width")
	}
}

// conjF32 negates every odd lane of an interleaved complex64 buffer. Over the
// real view because that is the form a vector kernel takes: a multiply by a
// constant {1,-1,1,-1} pattern.
func conjF32(dst, src []float32) {
	for i := 0; i+1 < len(src); i += 2 {
		dst[i], dst[i+1] = src[i], -src[i+1]
	}
}

// conjF64 is conjF32 at the wider width.
func conjF64(dst, src []float64) {
	for i := 0; i+1 < len(src); i += 2 {
		dst[i], dst[i+1] = src[i], -src[i+1]
	}
}

// VCAbs writes dst[i] = |src[i]|, the magnitude of each complex value, over
// min(len(dst), len(src)) elements.
//
// F is normally C's component type; Go cannot say so, so a mismatched pair is
// legal, correct and slow. The arithmetic happens at C's component width and
// the result is converted, so VCAbs[complex64, float64] is the float32 answer
// widened rather than a better one.
//
// At complex128 there is nothing wider to accumulate the squares in, so a
// magnitude above about 1.3e154 overflows where math.Hypot would not. Hypot has
// no vector form, so the naive expression is the contract at both widths.
func VCAbs[C Complex, F Float](dst []F, src []C) {
	switch s := any(src).(type) {
	case []complex64:
		vcabs64(dst, s)
	case []complex128:
		vcabs128(dst, s)
	default:
		panic("dsp: VCAbs: no kernel for this width")
	}
}

func vcabs64[F Float](dst []F, src []complex64) {
	if d, ok := any(dst).([]float32); ok {
		f32.CAbs(d, src)
		return
	}
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = F(float32(math.Sqrt(float64(float32(re*re) + float32(im*im)))))
	}
}

func vcabs128[F Float](dst []F, src []complex128) {
	if d, ok := any(dst).([]float64); ok {
		f64.CAbs(d, src)
		return
	}
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = F(math.Sqrt(re*re + im*im))
	}
}

// VCMulReal multiplies each complex sample by the matching real value, which is
// how a window is applied to a complex signal. It processes
// min(len(dst), len(src), len(w)) elements.
//
// F and C pair the same way they do for VCAbs, with the same consequences.
func VCMulReal[C Complex, F Float](dst, src []C, w []F) {
	switch d := any(dst).(type) {
	case []complex64:
		vcmulRealC64(d, any(src).([]complex64), w)
	case []complex128:
		vcmulRealC128(d, any(src).([]complex128), w)
	default:
		panic("dsp: VCMulReal: no kernel for this width")
	}
}

func vcmulRealC64[F Float](dst, src []complex64, w []F) {
	if ww, ok := any(w).([]float32); ok {
		f32.CMulReal(dst, src, ww)
		return
	}
	n := min(len(dst), len(src), len(w))
	for i, v := range src[:n] {
		m := float32(w[i])
		dst[i] = complex(real(v)*m, imag(v)*m)
	}
}

func vcmulRealC128[F Float](dst, src []complex128, w []F) {
	if ww, ok := any(w).([]float64); ok {
		f64.CMulReal(dst, src, ww)
		return
	}
	n := min(len(dst), len(src), len(w))
	for i, v := range src[:n] {
		m := float64(w[i])
		dst[i] = complex(real(v)*m, imag(v)*m)
	}
}

// VCMagSq writes dst[i] = |src[i]|^2, the squared magnitude of each complex
// value, over min(len(dst), len(src)) elements.
//
// It is VCAbs without the square root, and cheaper for it. Reach for it wherever
// the magnitudes are only compared or accumulated -- power detection, an AGC, an
// energy measure -- since the square root is monotonic and buys nothing there.
// F and C pair as they do for VCAbs, with the same consequences, except that
// there is no square root to overflow: the complex128 form squares at float64
// and so saturates to +Inf above about 1.3e154 rather than earlier.
func VCMagSq[C Complex, F Float](dst []F, src []C) {
	switch s := any(src).(type) {
	case []complex64:
		vcmagsq64(dst, s)
	case []complex128:
		vcmagsq128(dst, s)
	default:
		panic("dsp: VCMagSq: no kernel for this width")
	}
}

func vcmagsq64[F Float](dst []F, src []complex64) {
	if d, ok := any(dst).([]float32); ok {
		f32.CMagSq(d, src)
		return
	}
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = F(float32(re*re) + float32(im*im))
	}
}

func vcmagsq128[F Float](dst []F, src []complex128) {
	if d, ok := any(dst).([]float64); ok {
		f64.CMagSq(d, src)
		return
	}
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = F(float64(re*re) + float64(im*im))
	}
}
