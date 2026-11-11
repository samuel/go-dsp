//go:build amd64

package pcm

//go:generate go run -modfile=../tools.mod pcm_avo_amd64.go -out pcm_avo_amd64.s -stubs pcm_avo_stub_amd64.go

// The run-time dispatch over the generated leaves, used by the default build
// and, under GOEXPERIMENT=simd, on CPUs archsimd cannot target. It is in Go
// rather than the assembly so that tests can drive each leaf directly.

func u8ToF32Asm(dst []float32, src []byte) {
	switch {
	case useAVX:
		u8ToF32AVX(dst, src)
	case useSSE4:
		u8ToF32SSE4(dst, src)
	default:
		u8ToF32SSE2(dst, src)
	}
}

func i8ToF32Asm(dst []float32, src []byte) {
	switch {
	case useAVX:
		i8ToF32AVX(dst, src)
	case useSSE4:
		i8ToF32SSE4(dst, src)
	default:
		i8ToF32SSE2(dst, src)
	}
}

func i16LEToF32Asm(dst []float32, src []byte) {
	switch {
	case useAVX:
		i16LEToF32AVX(dst, src)
	case useSSE4:
		i16LEToF32SSE4(dst, src)
	default:
		i16LEToF32SSE2(dst, src)
	}
}

func i16LEToF64Asm(dst []float64, src []byte) {
	switch {
	case useAVX:
		i16LEToF64AVX(dst, src)
	case useSSE4:
		i16LEToF64SSE4(dst, src)
	default:
		i16LEToF64SSE2(dst, src)
	}
}

func f32ToI16LEAsm(dst []byte, src []float32) {
	if useAVX {
		f32ToI16LEAVX(dst, src)
		return
	}
	f32ToI16LESSE2(dst, src)
}
