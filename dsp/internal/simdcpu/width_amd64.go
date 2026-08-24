//go:build goexperiment.simd && amd64

package simdcpu

import (
	"os"
	"simd/archsimd"
	"strconv"

	"github.com/samuel/go-dsp/dsp/internal/cpu"
)

// width is the widest vector, in bits, that the archsimd implementations
// should use: the CPU's widest supported vector except where a wider one
// measures slower (see avx512Downclocks), overridable with the X86VECTOR
// environment variable.
//
// Zero means no vector path is usable and callers must run scalar code, which
// is not "use 128-bit vectors": archsimd compiles even its 128-bit operations
// to VEX encodings, so everything requires AVX. Nothing in the toolchain
// enforces that — GOEXPERIMENT=simd builds at GOAMD64=v1 — so a pre-AVX CPU
// faults with SIGILL rather than running slowly.
var width int

func init() {
	switch {
	case archsimd.X86.AVX512():
		width = 512
	case archsimd.X86.AVX():
		width = 256
	default:
		// No AVX, so no archsimd at all. See the note above.
		width = 0
	}
	if width == 512 && avx512Downclocks(cpu.X86Signature()) {
		width = 256
	}
	if w, err := strconv.Atoi(os.Getenv("X86VECTOR")); err == nil {
		switch {
		case w >= 512 && archsimd.X86.AVX512():
			width = 512
		case w >= 256 && archsimd.X86.AVX():
			width = 256
		case w >= 128 && archsimd.X86.AVX():
			width = 128
		case w >= 0:
			width = 0
		}
	}
}

// Width returns the widest archsimd vector, in bits, that may be used. Zero
// means no vector path is usable and callers must run scalar code, including
// archsimd.ClearAVXUpperBits.
func Width() int { return width }

// avx512Downclocks reports whether 512-bit instructions pull the core clock
// down far enough that the 256-bit path finishes sooner despite issuing twice
// as many instructions.
//
// Skylake-SP and its Cascade Lake and Cooper Lake refreshes (family 6, model
// 0x55) have a two-level AVX-512 frequency license with a large step, and the
// low-core-count SKUs have only one 512-bit FMA unit to win throughput back
// with. On a Xeon Silver 4210 scaling 16K floats the core settles at ~1.6GHz on
// the 512-bit path against ~2.5GHz on the 256-bit one, which wins by ~25%. Ice
// Lake and later, and AMD Zen 4/5, narrowed or removed the penalty and keep
// the 512-bit path.
func avx512Downclocks(sig cpu.Signature) bool {
	return sig.Vendor == "GenuineIntel" && sig.Family == 6 && sig.Model == 0x55
}
