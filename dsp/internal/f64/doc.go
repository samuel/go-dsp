// Package f64 holds the float64 and complex128 kernels behind the generic vector
// functions in dsp.
//
// complex128 is two float64 with no padding, so a complex kernel is a real
// kernel over a reinterpreted slice and lives at the same element width;
// archsimd likewise has float vector types and no complex ones.
package f64
