package f32

import "github.com/samuel/go-dsp/dsp/internal/simdcpu"

// The CPU flags this package dispatches on. simdcpu does the detection; these
// are variables rather than calls for the reason given in dsp/encoding's cpu.go.
//
// math32_arm.s reads ·haveNEON+0(SB) and ·useVector+0(SB), and the tests toggle
// all of them -- TestSIMDPreAVXFallback sets simdWidth directly to drive the
// scalar path on a machine that has AVX.
//
// useAVX means AVX1 and nothing more, so math32_avo_amd64.go must not emit an
// AVX2 encoding: VBROADCASTSS's memory-source form is AVX1, its register-source
// form AVX2, and the difference is a SIGILL on any pre-Haswell CPU. Check the
// "// Requires:" lines in math32_asm_amd64.s after regenerating.
var (
	useAVX    = simdcpu.HasAVX()
	haveNEON  = simdcpu.HaveNEON()
	useVector = simdcpu.UseVector()
	simdWidth = simdcpu.Width()
)

// haveSIMD reports whether the archsimd paths (including
// archsimd.ClearAVXUpperBits) may be executed at all. archsimd is VEX-encoded
// even at 128 bits, so a zero width means scalar, not narrower.
func haveSIMD() bool { return simdWidth > 0 }
