//go:build 386 || amd64 || arm || arm64

package convert

// The assembly entry points, declared in one place so the set of them is
// visible and asm_fallback.go can mirror it exactly.
//
// Each has a pure-Go reference of the same name without the Asm suffix, which
// defines its behavior; on an architecture with no assembly the stub is a JMP to
// that reference.

//go:noescape
func u8ToI16Asm(dst []int16, src []byte)

//go:noescape
func u8ToI16LEAsm(dst, src []byte)

//go:noescape
func i8ToF32Asm(dst []float32, src []byte)

//go:noescape
func f32ToI16Asm(dst []int16, src []float32)

//go:noescape
func f32ToI16LEAsm(dst []byte, src []float32)

//go:noescape
func i16ToI16LEAsm(dst []byte, src []int16)

//go:noescape
func i16LEToF64Asm(dst []float64, src []byte)

//go:noescape
func i16LEToF32Asm(dst []float32, src []byte)
