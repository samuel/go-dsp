//go:build !goexperiment.simd

package pcm

// Without GOEXPERIMENT=simd the vector kernels are the generated assembly,
// dispatched on the CPU's features in pcm_asm_amd64.go.

func u8ToF32(dst []float32, src []byte)    { u8ToF32Asm(dst, src) }
func u8ToF64(dst []float64, src []byte)    { u8ToFloatRef(dst, src) }
func i8ToF32(dst []float32, src []byte)    { i8ToF32Asm(dst, src) }
func i8ToF64(dst []float64, src []byte)    { i8ToFloatRef(dst, src) }
func i16LEToF32(dst []float32, src []byte) { i16LEToF32Asm(dst, src) }
func i16LEToF64(dst []float64, src []byte) { i16LEToF64Asm(dst, src) }
func i24LEToF32(dst []float32, src []byte) { i24LEToFloatRef(dst, src) }
func i24LEToF64(dst []float64, src []byte) { i24LEToFloatRef(dst, src) }
func i32LEToF32(dst []float32, src []byte) { i32LEToFloatRef(dst, src) }
func i32LEToF64(dst []float64, src []byte) { i32LEToFloatRef(dst, src) }
func f32LEToF32(dst []float32, src []byte) { f32LEToFloatRef(dst, src) }
func f32LEToF64(dst []float64, src []byte) { f32LEToFloatRef(dst, src) }
func f64LEToF32(dst []float32, src []byte) { f64LEToFloatRef(dst, src) }
func f64LEToF64(dst []float64, src []byte) { f64LEToFloatRef(dst, src) }
func alawToF32(dst []float32, src []byte)  { alawToFloatRef(dst, src) }
func alawToF64(dst []float64, src []byte)  { alawToFloatRef(dst, src) }
func mulawToF32(dst []float32, src []byte) { mulawToFloatRef(dst, src) }
func mulawToF64(dst []float64, src []byte) { mulawToFloatRef(dst, src) }
