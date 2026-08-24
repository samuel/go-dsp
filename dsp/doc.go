// Package dsp is the primary API for the go-dsp SDR library: generic vector
// operations (VScale, VAdd, ...), FM demodulation, IIR/biquad/FIR filters and
// decimators, fixed-size FFT, Goertzel tone detection, sliding DFT,
// transfer-function evaluation, window functions and interpolation.
//
// Functions are generic over the sample width (float32 | float64, and the
// complex families); elementwise binary operations are three-operand
// f(dst, a, b), reductions return a value, and 1:1 operations process
// min(dst, src). Stateful types have a validating constructor and a Reset,
// and none are safe for concurrent use.
//
// The performance-critical kernels are vectorized for 386, amd64, arm and
// arm64 with a pure-Go fallback on every other architecture, so the same
// source builds and runs scalar on riscv64, ppc64le, s390x and wasm.
package dsp
