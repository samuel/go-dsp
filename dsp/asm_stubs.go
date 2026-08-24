//go:build 386 || amd64 || arm || arm64

package dsp

// The assembly entry points, declared in one place so that the set of them is
// visible and so that asm_fallback.go can mirror it exactly.
//
// Every one has a pure-Go reference of the same name without the Asm suffix,
// which is the definition of its behavior; on an architecture with no assembly
// for it, the stub is a JMP to that reference. The exported functions on top are
// ordinary Go wrappers, small enough to inline, which is what keeps the API
// buildable on architectures with no assembly at all -- see asm_fallback.go.

func fastAtan2Asm(y, x float32) float32
func fastAtan2FineAsm(y, x float32) float32

func rotate90Asm(samples []complex64)

func fmDemodulateAsm(f *FMDemod, dst []float32, src []complex64)

func boxcarDecimateAsm(f *BoxcarDecimator, dst, src []complex64) int
func rationalBoxcarDecimateAsm(f *RationalBoxcarDecimator, dst, src []float32) int
