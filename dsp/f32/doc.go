// Package f32 holds the float32 and complex64 kernels behind the generic vector
// functions in dsp.
//
// complex64 is two float32 with no padding, so a complex kernel is a real
// kernel over a reinterpreted slice and lives at the same element width;
// archsimd likewise has float vector types and no complex ones.
//
// Call these directly only to skip dsp's generic dispatch, a fixed ~1.2ns per
// call that matters only below about 64 elements. Everything else should use
// dsp.VScale et al.
package f32
