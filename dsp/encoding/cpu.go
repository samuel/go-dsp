package encoding

import "github.com/samuel/go-dsp/dsp/internal/simdcpu"

// The CPU flags this package dispatches on; the detection is simdcpu's.
//
// A Go assembly reference ·name(SB) resolves within its own package, so a flag
// the assembly reads must be declared where the assembly lives:
// conversion_amd64.s reads ·useSSE4(SB), conversion_arm.s ·haveNEON+0(SB). And
// the tests toggle them to exercise both paths on one machine, which only works
// with a per-package copy — hence dsp/f32's near-identical file.
var (
	useSSE4   = simdcpu.HasSSE4()
	haveNEON  = simdcpu.HaveNEON()
	simdWidth = simdcpu.Width()
)

// haveSIMD reports whether the archsimd paths, including
// archsimd.ClearAVXUpperBits, may be executed at all. archsimd is VEX-encoded
// even for its 128-bit types, so a zero width means scalar and not narrower.
func haveSIMD() bool { return simdWidth > 0 }
