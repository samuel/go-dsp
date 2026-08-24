package dsp

import (
	"github.com/samuel/go-dsp/dsp/f32"
	"github.com/samuel/go-dsp/dsp/f64"
)

// Elementwise vector arithmetic over real samples. The naming, three-operand
// and dispatch rules are in the package comment; the complex family is in
// vector_complex.go.
//
// The type switches panic in their default arm rather than falling through and
// writing nothing, which is what a constraint member added without a matching
// kernel would otherwise do. TestVectorCoversEveryWidth instantiates every
// width, so the gap shows up there first.

// VScale writes dst[i] = src[i] * k, over min(len(dst), len(src)) elements.
// The slices may be the same.
func VScale[F Float](dst, src []F, k F) {
	switch d := any(dst).(type) {
	case []float32:
		f32.Scale(d, any(src).([]float32), float32(k))
	case []float64:
		f64.Scale(d, any(src).([]float64), float64(k))
	default:
		panic("dsp: VScale: no kernel for this width")
	}
}

// VAdd writes dst[i] = a[i] + b[i], over min(len(dst), len(a), len(b))
// elements. Any of the slices may be the same, so VAdd(x, x, y) accumulates y
// into x.
func VAdd[F Float](dst, a, b []F) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v + b[i]
	}
}

// VSub writes dst[i] = a[i] - b[i], under VAdd's length and aliasing rules.
func VSub[F Float](dst, a, b []F) {
	n := min(len(dst), len(a), len(b))
	for i, v := range a[:n] {
		dst[i] = v - b[i]
	}
}

// VMul writes dst[i] = a[i] * b[i], under VAdd's length and aliasing rules.
// This is the elementwise product, which is how a window is applied to a real
// signal; VScale is the one to reach for when the multiplier is a scalar.
func VMul[F Float](dst, a, b []F) {
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
