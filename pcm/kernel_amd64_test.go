package pcm

import "testing"

// TestLeavesMatchReference drives every generated leaf directly, rather than
// only the one the dispatchers pick on this CPU.
func TestLeavesMatchReference(t *testing.T) {
	type leaf struct {
		isa string
		ok  bool
	}
	avx, sse4, sse2 := leaf{"AVX", useAVX}, leaf{"SSE4", useSSE4}, leaf{"SSE2", true}
	run := func(l leaf, name string, fn func(t *testing.T)) {
		t.Run(name+l.isa, func(t *testing.T) {
			if !l.ok {
				t.Skipf("%s not available", l.isa)
			}
			fn(t)
		})
	}
	for _, c := range []struct {
		l           leaf
		u8, i8, i16 func([]float32, []byte)
		i16F64      func([]float64, []byte)
	}{
		{avx, u8ToF32AVX, i8ToF32AVX, i16LEToF32AVX, i16LEToF64AVX},
		{sse4, u8ToF32SSE4, i8ToF32SSE4, i16LEToF32SSE4, i16LEToF64SSE4},
		{sse2, u8ToF32SSE2, i8ToF32SSE2, i16LEToF32SSE2, i16LEToF64SSE2},
	} {
		run(c.l, "u8ToF32", func(t *testing.T) {
			checkDecodeKernel(t, decodeKernel[float32]{"u8ToF32", 1, c.u8, u8ToFloatRef[float32]})
		})
		run(c.l, "i8ToF32", func(t *testing.T) {
			checkDecodeKernel(t, decodeKernel[float32]{"i8ToF32", 1, c.i8, i8ToFloatRef[float32]})
		})
		run(c.l, "i16LEToF32", func(t *testing.T) {
			checkDecodeKernel(t, decodeKernel[float32]{"i16LEToF32", 2, c.i16, i16LEToFloatRef[float32]})
		})
		run(c.l, "i16LEToF64", func(t *testing.T) {
			checkDecodeKernel(t, decodeKernel[float64]{"i16LEToF64", 2, c.i16F64, i16LEToFloatRef[float64]})
		})
	}
	for _, c := range []struct {
		l  leaf
		fn func([]byte, []float32)
	}{
		{avx, f32ToI16LEAVX},
		{sse2, f32ToI16LESSE2},
	} {
		run(c.l, "f32ToI16LE", func(t *testing.T) {
			checkEncodeKernel(t, encodeKernel[float32]{"f32ToI16LE", 2, c.fn, floatToI16LERef[float32]})
		})
	}
}
