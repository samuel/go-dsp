package pcm

import "testing"

// simdTest runs fn on every path the CPU flags can select: as detected, then
// with AVX off, then with SSE4.1 off too. Under GOEXPERIMENT=simd the first
// run takes the archsimd kernels and the others the generated assembly.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	sse4, avx, width := useSSE4, useAVX, simdWidth
	defer func() { useSSE4, useAVX, simdWidth = sse4, avx, width }()

	t.Run("detected", fn)
	simdWidth = 0
	if avx {
		useAVX = false
		t.Run("noavx", fn)
	}
	if sse4 {
		useSSE4 = false
		t.Run("nosse4", fn)
	}
}
