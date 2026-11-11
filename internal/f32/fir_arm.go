package f32

// The NEON leaves are in fir_arm.s. A strided FIR, or a CPU without NEON,
// runs the reference.

func firF32(dst, work, taps []float32, stride int) {
	if haveNEON && stride == 1 {
		firF32NEON(dst, work, taps)
		return
	}
	firF32Go(dst, work, taps, stride)
}

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	if haveNEON && stride == 1 {
		cfirRealNEON(dst, work, taps)
		return
	}
	cfirRealGo(dst, work, taps, stride)
}

func cfir(dst, work, taps []complex64, stride int) {
	if haveNEON && stride == 1 {
		cfirNEON(dst, work, taps)
		return
	}
	cfirGo(dst, work, taps, stride)
}

func dotF32(x, c []float32) float32 {
	if haveNEON {
		return dotF32NEON(x, c)
	}
	return dotF32Go(x, c)
}

func cdotReal(x []complex64, c []float32) complex64 {
	if haveNEON {
		return cdotRealNEON(x, c)
	}
	return cdotRealGo(x, c)
}

// firF32NEON is FIR with stride 1 over len(dst) outputs. taps must not be
// empty when dst is not.
//
//go:noescape
func firF32NEON(dst, work, taps []float32)

//go:noescape
func cfirRealNEON(dst, work []complex64, taps []float32)

//go:noescape
func cfirNEON(dst, work, taps []complex64)

// dotF32NEON is Dot over len(c) elements; x must be at least as long.
//
//go:noescape
func dotF32NEON(x, c []float32) float32

//go:noescape
func cdotRealNEON(x []complex64, c []float32) complex64
