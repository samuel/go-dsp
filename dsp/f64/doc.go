// Package f64 holds the float64 and complex128 kernels behind the generic vector
// functions in dsp.
//
// complex128 is two float64 with no padding, so a complex kernel is a real
// kernel over a reinterpreted slice and lives at the same element width;
// archsimd likewise has float vector types and no complex ones.
//
// Call these directly only to skip dsp's generic dispatch, a fixed ~1.2ns per
// call that matters only below about 64 elements. Everything else should use
// dsp.VScale et al.
package f64
