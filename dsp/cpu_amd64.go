package dsp

import (
	"github.com/samuel/go-dsp/dsp/internal/cpu"
)

var (
	useSSE4 bool
	useAVX2 bool
	useSSE2 bool
)

func init() {
	useSSE4 = cpu.X86.HasSSE41
	useAVX2 = cpu.X86.HasAVX
	useSSE2 = cpu.X86.HasSSE2
}
