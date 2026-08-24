//go:build !goexperiment.simd || !amd64

package simdcpu

// Width returns the widest archsimd vector, in bits, that may be used. Zero
// means no vector path is usable and callers must run scalar code, which is the
// answer everywhere archsimd's amd64 width selection does not apply.
func Width() int { return 0 }
