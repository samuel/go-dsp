package pcm

import "samuelks.com/dsp/internal/simdcpu"

// The CPU flags this package dispatches on; the detection is simdcpu's.
//
// They are variables in this package because a Go assembly reference
// ·name(SB) resolves within its own package, and pcm_arm.s reads
// ·haveNEON+0(SB). The tests toggle them to drive every path on one machine.
//
// useAVX means AVX1 and nothing more, so pcm_avo_amd64.go must not emit an
// AVX2 encoding; check the "// Requires:" lines in pcm_avo_amd64.s after
// regenerating.
var (
	useSSE4   = simdcpu.HasSSE4()
	useAVX    = simdcpu.HasAVX()
	haveNEON  = simdcpu.HaveNEON()
	simdWidth = simdcpu.Width()
)

// haveSIMD reports whether the archsimd paths, including
// archsimd.ClearAVXUpperBits, may be executed at all. archsimd is VEX-encoded
// even for its 128-bit types, so a zero width means scalar and not narrower.
func haveSIMD() bool { return simdWidth > 0 }
