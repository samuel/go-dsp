//go:build !386 && !amd64 && !arm && !arm64

package dsp

// The pure-Go stand-ins for asm_stubs.go, so that the package builds and runs
// on an architecture nobody has written assembly for -- riscv64, ppc64le,
// s390x, wasm and whatever comes next. Each one calls the reference
// implementation that defines its behavior, which is also what the assembly
// falls back to on architectures where only some functions are vectorized.

func fastAtan2Asm(y, x float32) float32     { return fastAtan2(y, x) }
func fastAtan2FineAsm(y, x float32) float32 { return fastAtan2Fine(y, x) }

func rotate90Asm(samples []complex64) { rotate90(samples) }

func fmDemodulateAsm(f *FMDemod, dst []float32, src []complex64) {
	fmDemodulate(f, dst, src)
}

func boxcarDecimateAsm(f *BoxcarDecimator, dst, src []complex64) int {
	return boxcarDecimate(f, dst, src)
}

func rationalBoxcarDecimateAsm(f *RationalBoxcarDecimator, dst, src []float32) int {
	return rationalBoxcarDecimate(f, dst, src)
}
