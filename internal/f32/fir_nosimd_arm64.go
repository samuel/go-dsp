//go:build arm64 && !goexperiment.simd

package f32

import "samuelks.com/dsp/internal/view"

// The FIR leaves vectorize across outputs, which needs consecutive outputs to
// read consecutive samples; a stride above 1 (a decimator) runs the reference.
// The assembly is in fir_nosimd_arm64.s.

func firF32(dst, work, taps []float32, stride int) {
	if stride != 1 {
		firF32Go(dst, work, taps, stride)
		return
	}
	firNEON(dst, work, taps, 1)
}

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	if stride != 1 {
		cfirRealGo(dst, work, taps, stride)
		return
	}
	firNEON(view.C64ToF32(dst), view.C64ToF32(work), taps, 2)
}

func cfir(dst, work, taps []complex64, stride int) {
	if stride != 1 {
		cfirGo(dst, work, taps, stride)
		return
	}
	cfirNEON(dst, work, taps)
}

func dotF32(x, c []float32) float32 { return dotNEON(x, c) }

func cdotReal(x []complex64, c []float32) complex64 { return cdotRealNEON(x, c) }

// firNEON is FIR over len(dst) outputs with the taps reaching back step
// elements at a time. work must hold len(dst)+(len(taps)-1)*step elements.
//
//go:noescape
func firNEON(dst, work, taps []float32, step int)

// cfirNEON is CFIR with a stride of 1.
//
//go:noescape
func cfirNEON(dst, work, taps []complex64)

//go:noescape
func dotNEON(x, c []float32) float32

//go:noescape
func cdotRealNEON(x []complex64, c []float32) complex64
