//go:build !arm && !arm64

package encoding

// Scalar implementation of the byte-to-float conversion, the fallback wherever
// neither archsimd nor hand-written assembly is available: 386 and any future
// port. amd64 builds it too, but only tests reach it — conversion_avo_amd64.s
// covers every CPU there.

// u8ToF32 converts unsigned 8-bit samples to float32.
//
// Subtracting the bias in int rather than byte arithmetic keeps the
// intermediate exact for every input; each result is a small integer, so the
// float32 conversion is exact too and every implementation agrees bit for bit,
// which is why the tests can compare exactly.
func u8ToF32(dst []float32, src []byte) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for i, v := range src {
		dst[i] = float32(int(v) - 128)
	}
}
