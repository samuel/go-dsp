package simdcpu

import (
	"os"

	"github.com/samuel/go-dsp/dsp/internal/cpu"
)

func init() {
	// Initialize takes a GODEBUG-style string and honors cpu.* fields in it, so
	// passing the real GODEBUG lets a feature be turned off from the
	// environment, e.g. GODEBUG=cpu.avx=off.
	cpu.Initialize(os.Getenv("GODEBUG"))
}

// HasSSE4 reports whether SSE4.1 is available.
func HasSSE4() bool { return cpu.X86.HasSSE41 }

// HasAVX reports whether AVX is available. It promises AVX1 only: the assembly
// it guards is reached on Sandy Bridge and Bulldozer, so nothing behind it may
// use an AVX2 encoding.
func HasAVX() bool { return cpu.X86.HasAVX }
