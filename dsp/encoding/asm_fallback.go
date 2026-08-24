//go:build !386 && !amd64 && !arm && !arm64

package encoding

// The mirror of asm_stubs.go for architectures with no assembly: every entry
// point routes to its pure-Go reference. The two files must declare the same
// names with the same signatures; TestAsmStubsAndFallbackAgree enforces that by
// parsing both.

func u8ToI16Asm(dst []int16, src []byte)      { u8ToI16(dst, src) }
func u8ToI16LEAsm(dst, src []byte)            { u8ToI16LE(dst, src) }
func i8ToF32Asm(dst []float32, src []byte)    { i8ToF32(dst, src) }
func f32ToI16Asm(dst []int16, src []float32)  { f32ToI16(dst, src) }
func f32ToI16LEAsm(dst []byte, src []float32) { f32ToI16LE(dst, src) }
func i16ToI16LEAsm(dst []byte, src []int16)   { i16ToI16LE(dst, src) }
func i16LEToF64Asm(dst []float64, src []byte) { i16LEToF64(dst, src) }
func i16LEToF32Asm(dst []float32, src []byte) { i16LEToF32(dst, src) }
