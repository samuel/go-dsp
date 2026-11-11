//go:build amd64 && !goexperiment.simd

package f32

func firF32(dst, work, taps []float32, stride int) { firF32Asm(dst, work, taps, stride) }

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	cfirRealAsm(dst, work, taps, stride)
}

func cfir(dst, work, taps []complex64, stride int) { cfirAsm(dst, work, taps, stride) }

func dotF32(x, c []float32) float32 { return dotF32Asm(x, c) }

func cdotReal(x []complex64, c []float32) complex64 { return cdotRealAsm(x, c) }
