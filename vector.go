package dsp

import (
	"samuelks.com/dsp/internal/f32"
	"samuelks.com/dsp/internal/f64"
)

// Elementwise vector arithmetic. VScale, VAdd, VSub and VMul take any Sample;
// the reductions are real only, and the operations that only make sense on
// complex samples are in vector_complex.go. The naming, three-operand and
// dispatch rules are in the package comment.
//
// The type switches panic in their default arm rather than falling through and
// writing nothing, which is what a constraint member added without a matching
// kernel would otherwise do. TestVectorCoversEveryWidth instantiates every
// width, so the gap shows up there first.

// VScale writes dst[i] = src[i] * k, over min(len(dst), len(src)) elements.
// The slices may be the same. A complex k rotates as well as scales; use
// VScaleReal for a real gain on a complex signal, which is a plain per-lane
// multiply.
func VScale[T Sample](dst, src []T, k T) {
	switch d := any(dst).(type) {
	case []float32:
		f32.Scale(d, any(src).([]float32), any(k).(float32))
	case []float64:
		f64.Scale(d, any(src).([]float64), any(k).(float64))
	case []complex64:
		vscaleC64(d, any(src).([]complex64), any(k).(complex64))
	case []complex128:
		vscaleC128(d, any(src).([]complex128), any(k).(complex128))
	default:
		panic("dsp: VScale: no kernel for this width")
	}
}

// VAdd writes dst[i] = a[i] + b[i], over min(len(dst), len(a), len(b))
// elements. Any of the slices may be the same, so VAdd(x, x, y) accumulates y
// into x.
func VAdd[T Sample](dst, a, b []T) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v + b[i]
	}
}

// VSub writes dst[i] = a[i] - b[i], under VAdd's length and aliasing rules.
func VSub[T Sample](dst, a, b []T) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v - b[i]
	}
}

// VMul writes dst[i] = a[i] * b[i], under VAdd's length and aliasing rules.
// This is the elementwise product, the complex product for complex samples.
// A real window is applied to a real signal with it and to a complex signal
// with VMulReal; VScale is the one to reach for when the multiplier is a
// scalar.
func VMul[T Sample](dst, a, b []T) {
	switch d := any(dst).(type) {
	case []float32:
		vmulReal(d, any(a).([]float32), any(b).([]float32))
	case []float64:
		vmulReal(d, any(a).([]float64), any(b).([]float64))
	case []complex64:
		vmulC64(d, any(a).([]complex64), any(b).([]complex64))
	case []complex128:
		vmulC128(d, any(a).([]complex128), any(b).([]complex128))
	default:
		panic("dsp: VMul: no kernel for this width")
	}
}

func vmulReal[F Float](dst, a, b []F) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v * b[i]
	}
}

// VMax returns the largest value in src, or -Inf for an empty slice. Which
// value is returned when src contains a NaN is unspecified.
func VMax[F Float](src []F) F {
	switch s := any(src).(type) {
	case []float32:
		return F(f32.Max(s))
	case []float64:
		return F(f64.Max(s))
	}
	panic("dsp: VMax: no kernel for this width")
}

// VMin returns the smallest value in src, or +Inf for an empty slice. Which
// value is returned when src contains a NaN is unspecified.
func VMin[F Float](src []F) F {
	switch s := any(src).(type) {
	case []float32:
		return F(f32.Min(s))
	case []float64:
		return F(f64.Min(s))
	}
	panic("dsp: VMin: no kernel for this width")
}

// VSum returns the sum of src, or 0 for an empty slice.
//
// The summation order is not left to right and is not part of the contract: the
// kernels use several independent accumulators, which is both more accurate on
// a long input and the only loop structure that vectorizes. See f32.Sum.
func VSum[F Float](src []F) F {
	switch s := any(src).(type) {
	case []float32:
		return F(f32.Sum(s))
	case []float64:
		return F(f64.Sum(s))
	}
	panic("dsp: VSum: no kernel for this width")
}

// VMaxIdx returns the largest value in src and its index, or (-Inf, -1) for an
// empty slice. On a tie the lowest index wins. Which value is returned when src
// contains a NaN is unspecified, as for VMax.
func VMaxIdx[F Float](src []F) (F, int) {
	switch s := any(src).(type) {
	case []float32:
		v, i := f32.MaxIdx(s)
		return F(v), i
	case []float64:
		v, i := f64.MaxIdx(s)
		return F(v), i
	}
	panic("dsp: VMaxIdx: no kernel for this width")
}
