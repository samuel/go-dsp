package pcm

import "samuelks.com/dsp/internal/simdcpu"

// The CPU features that select a kernel. Package simdcpu detects them.
//
// They are variables in this package for two reasons. Assembly can only refer
// to a symbol of its own package, and pcm_arm.s reads ·haveNEON+0(SB). And
// the tests change them, to run every code path on one machine.
//
// useAVX promises AVX1 only, so the generator in pcm_avo_amd64.go must not
// emit an AVX2 instruction. After regenerating, check the "// Requires:"
// lines in pcm_avo_amd64.s.
var (
	useSSE4   = simdcpu.HasSSE4()
	useAVX    = simdcpu.HasAVX()
	haveNEON  = simdcpu.HaveNEON()
	simdWidth = simdcpu.Width()
)

// haveSIMD reports whether any archsimd code may run, including
// archsimd.ClearAVXUpperBits. archsimd uses VEX encoding even for its 128-bit
// types, so a CPU without AVX cannot run a narrower archsimd path and must
// use the scalar code.
func haveSIMD() bool { return simdWidth > 0 }
