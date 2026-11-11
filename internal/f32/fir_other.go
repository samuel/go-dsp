//go:build !arm && !amd64 && !arm64

package f32

// No vector unit to use: the references are the kernels.

func firF32(dst, work, taps []float32, stride int) { firF32Go(dst, work, taps, stride) }

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	cfirRealGo(dst, work, taps, stride)
}

func cfir(dst, work, taps []complex64, stride int) { cfirGo(dst, work, taps, stride) }

func dotF32(x, c []float32) float32 { return dotF32Go(x, c) }

func cdotReal(x []complex64, c []float32) complex64 { return cdotRealGo(x, c) }
