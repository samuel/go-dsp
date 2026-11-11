package f32

import "testing"

// simdTest reruns fn with each CPU feature flag toggled off, so that both the
// SIMD and the fallback assembly paths are exercised on one machine.
//
// Only AVX is toggled here. SSE4 gates the sample codec kernels, which live
// in pcm and carry their own copy of this helper.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	if useAVX {
		t.Run("avx", fn)
		useAVX = false
		t.Run("noavx", fn)
		useAVX = true
	} else {
		t.Run("avx", func(t *testing.T) { t.Skip("avx not available") })
		t.Run("noavx", fn)
	}
}
