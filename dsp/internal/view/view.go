// Package view reinterprets a complex slice as the real pairs that back it,
// zero-copy. complex64 is two float32 with no padding and complex128 two
// float64, so the complex kernels in dsp are the real kernels over that view;
// the archsimd paths have no complex vector types and see the same pairs.
// The length contract survives the doubling, since min(2a, 2b) is 2*min(a, b).
package view

import "unsafe"

// C64ToF32 views a complex64 slice as float32 pairs.
func C64ToF32(s []complex64) []float32 {
	return unsafe.Slice((*float32)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2)
}

// C128ToF64 views a complex128 slice as float64 pairs.
func C128ToF64(s []complex128) []float64 {
	return unsafe.Slice((*float64)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*2)
}
